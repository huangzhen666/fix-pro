# Go 线上日志排障 Runbook

**日期：** 2026-10-07。对应 SPEC-012 / PLAN-012。日志输出使用 UTC，Grafana 显示时区可切换 Asia/Shanghai；反馈时间必须注明时区。

## 从反馈到根因

1. 收集请求参考号、发生时间、操作和受控业务 ID。客户端参考号意味着可能未收到服务端响应，不能据此认定请求已到应用。
2. Grafana → 灵狸护家 线上诊断 → 选环境和时间前后 15 分钟，填写 requestId；看 http.request.completed 的状态、errorCode、route、durationMs、身份、实例/版本。
3. 查同 ID 的 http.request.failed / http.panic、db.query.slow、dependency.attempt.completed，使用 operation、queryName、sqlState 和 caller 定位。安全摘要不包含 SQL 参数/个人资料。
4. 按组织和订单/工单 ID 查看业务事件，再用只读 SQL 核对数据库历史的 request_id/job_run_id。诊断事件可能缺失，数据库提交记录为事实来源。
5. 对照 version/gitCommit 查对应部署代码。静态 queryName 查询 `apps/server-go/api/diagnostic-queries.json`；运行时动态 SQL 未收入清单时使用 operation。源码调整后运行 `python3 scripts/generate-diagnostic-queries.py` 更新清单。
6. 未找到日志时检查客户端网络、代理状态和请求 ID、应用重启、采集探针与延迟。不要把缺失日志当作请求未到达的唯一证据。

固定查询示例（先用环境/时间范围约束）：

```logql
{service="fix-pro-api",env="production"} | json | __error__="" | requestId="REFERENCE"
{service="fix-pro-api",env="production"} | json | __error__="" | orgId="1" | orderId="123"
{service="fix-pro-api",env="production"} | json | __error__="" | workOrderId="456"
{service="fix-pro-api",env="production"} | json | __error__="" | jobRunId="JOB_REFERENCE"
```

输入必须按 LogQL 字符串规则编码；使用预置变量，不拼接未转义用户输入。运维有平台级权限，日志平台不对租户管理员开放。

## 应急本地日志与组件状态

以下在仓库根目录运行，实际生产替换为对应 Compose 项目和安全环境文件。不要输出环境文件、完整容器 inspect 或全部配置到公共渠道。

```bash
docker compose -f deploy/compose.yaml logs --since 30m server
docker compose -f deploy/compose.yaml logs --since 30m server | rg -F 'REFERENCE'
docker compose -f deploy/compose.yaml -f deploy/observability/compose.observability.yaml ps
docker compose -f deploy/compose.yaml -f deploy/observability/compose.observability.yaml logs --since 15m alloy loki log-probe
```

proxy 访问日志需配置服务端响应 X-Request-Id、上游状态/耗时/实例。请求未到 Go 的 502/504 使用代理参考号与时间定位；现有仓库未交付生产反向代理配置，部署时补齐。

## 常见故障

| 问题 | 检查顺序 | 处置与恢复确认 |
| --- | --- | --- |
| SQL 失败/约束冲突 | request.failed 的 queryName/operation/sqlState → 对应源码 → 状态与审计 | 修复实际数据/语义/代码；业务 409 与未知 500 分开，按原操作验证 |
| 连接池耗尽/锁等待 | db.pool.snapshot 的 inUse/open/wait 增量 → 数据库活动/事务时长 → 是否持有 Rows 等待网络 | 优先修复未关闭 Rows/长事务或超时；终止会话前确认影响，恢复后看等待增量下降 |
| 微信超时/异常 | attempt.completed 的状态、upstreamCode、耗时 → 网络/额度/密钥注入/实际资格 | 使用平台文档和受控重试，不打印 Code/Token；真实授权成功后确认错误率恢复 |
| Panic | requestId/jobRunId、版本、operation、stack、responseCommitted | 定位并修复/回滚；写出后 panic 客户端可能已经收到成功，先核对事务，不能盲目再次提交 |
| 派单冲突/状态异常 | orgId/workOrderId 事件、历史版本、操作者、幂等结果 | 刷新现状后操作，不覆盖已提交状态；用历史对照并验证后续履约 |
| 任务停跑/失败 | jobName + instanceId 的最近结束、job.failed/panic、jobRunId → 应用进程/部署静默窗口 | 恢复实例或修复查询；确认后续轮次结束、数量与关联事件，不人工重复验收 |
| 采集断流 | Prometheus up 和 probe 的 API/Loki/delivery → 代理授权/采集进度/服务标签 → Alloy dropped/retry 指标 | 恢复组件/访问；在有限本地轮转窗口内检查日志，必要重读可能重复，报告缺口 |
| 磁盘不足 | 主机磁盘、Loki/Alloy/Docker 卷 → retention/Compactor 状态 → 日志峰值与缓冲 | 扩容或按既定保留处理，不删除审计数据/共享卷；恢复组件与探针并确认删旧记录有效 |

受限只读 SQL（通过已有安全连接方式执行，参数按实际类型绑定；不输出 query 文本、地址或手机号）：

```sql
SELECT pid, state, wait_event_type, wait_event,
       now()-xact_start AS transaction_age
FROM pg_stat_activity
WHERE datname=current_database() AND pid<>pg_backend_pid()
ORDER BY xact_start NULLS LAST LIMIT 30;

SELECT pid, locktype, mode, granted
FROM pg_locks WHERE NOT granted LIMIT 30;

SELECT id, from_status, to_status, event_code, operator_type,
       operator_id, request_id, job_run_id, created_at
FROM work_order_status_history
WHERE org_id=$1 AND work_order_id=$2 ORDER BY created_at,id LIMIT 100;

SELECT id, action_code, resource_type, resource_id, request_id, created_at
FROM audit_log WHERE org_id=$1 AND request_id=$2 ORDER BY created_at,id LIMIT 100;
```

SQL 占位符需在 PostgreSQL 客户端/程序中实际绑定，不把 `$1` 原样粘贴成未准备的查询。

## 告警、发布与回滚

Grafana 通知受控根因/时间/路由和查询链接，不带原始请求/个人资料。默认低流量核心失败、5xx 比例、P95、panic、微信失败、任务与采集/磁盘分别判断；HTTP 只统计完成事件。共享测试环境的 event / env 标签不可当作生产事实。

日志链的 No Data/查询失败不算正常，无流量可由 probe 区分。实际通知 webhook、环境过滤、实例预期和部署静默窗口在上线验收中登记；单机观测栈整体宕机需要外部云监控/值守，不能依赖 Grafana 自报。

发布：先观测栈/访问控制与接收渠道，再向后兼容迁移、后端、前端；确认真实请求/业务/任务关联。回滚应用版本时保留新增历史字段，禁止默认 Down。回滚观测配置不停止 API，不删除卷。审计写入失败导致的回滚不能通过静默忽略审计修复。

记录故障参考号、版本、原因、操作、恢复时间和日志缺口；复测关键业务并确认恢复通知。首期保留生产 30 天、测试 7 天，审计保留不受 Loki 策略影响。日志级别设为 warn/error 会隐藏正常请求与成功业务事件，临时 debug 调整后应恢复 info。
