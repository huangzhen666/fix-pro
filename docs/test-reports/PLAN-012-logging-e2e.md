# PLAN-012 日志系统实施与验收记录

日期：2026-10-07。状态：应用实现及本地验证完成；集中日志平台交付配置，实际部署验收待完成。

对应 [计划](../plans/PLAN-012-Go后端日志系统与线上问题定位.md)、[规格](../specs/SPEC-012-Go后端日志系统与线上问题定位.md)、[排障 Runbook](../runbooks/production-logging.md)。本记录不代表生产已接入 Grafana 或已收到告警。

## 1. 基线与环境

- 工作区已有大量未提交的客户登录、媒体和前端改动；保留原有工作，在其上集成诊断，没有执行 reset、清理、提交或发布。
- 实施基线 HEAD：`5e1b797ee4b2da52c1a0d8346fd99b9d76f7c85b`。测试对象含未提交改动，HEAD 不能单独复现本次结果。
- macOS 26.6.2 / arm64；Go 1.27.1；Node 24；PostgreSQL 18。Go 依赖通过命令级 GOPROXY 下载，未修改全局 Go 配置。
- PostgreSQL 使用本次创建的独立临时实例、独立 `fixpro_logging_test` 库；故障注入测试额外检查库名，不连接共享开发或生产库。报告不保存 DSN。
- 本机没有 Docker，未启动 Alloy/Loki/Grafana；未提供真实微信环境、告警接收地址、生产代理或容量预算。
- 原版业务负载基线未保存，不能据当前结果证明 P95 增幅 ≤5% 或 CPU 增幅 ≤10%。

## 2. 实现范围

| 模块 | 交付 |
| --- | --- |
| Zap 底座 | Zap 1.28.0；JSON/console、级别与阈值、部署元信息、UTC、字段白名单、保留字段保护、限长、无采样、安全错误转换、Sync |
| HTTP | 共享诊断 context；ID 校验/生成/响应头/CORS；认证身份；路由模板；完成记录；Cause/Unwrap；安全根因；panic；httpsnoop 1.0.4 保留响应接口能力 |
| 数据库 | pgx tracer；SQL 指纹与代码操作位置；慢查询；失败根因安全 SQLSTATE；连接池快照；事务局部请求/任务 ID |
| 依赖与媒体 | 微信真实尝试事件、超时及失败类别；媒体大小/MIME/受控 ID；不输出 URL 凭据、响应原文或对象键 |
| 后台任务 | 三类任务独立 jobRunId、SYSTEM、结果/计数/耗时；取消/超时/panic；应用停止等待任务退出 |
| 业务 | 下单/幂等、订单确认/拒绝、派单/改派/收回、履约/完工/审核/验收、账号权限、SKU、媒体事件；成功在 Commit 后；工单事件绑定订单 ID |
| 审计与历史 | 高风险管理员及师傅密码/权限修改与审计同事务；会话撤销收口到事务；审计 request_id；迁移 017 增补五张历史/事件表关联字段 |
| 客户端 | 后台统一错误详情；小程序普通/上传/下载/登录异常参考号；区分服务端与客户端 ID；详情仅含受控诊断字段 |
| 维护入口 | 统一 Zap、安全错误、退出清理；管理员初始化生成密码写终端或显式 0600 文件，避免进入采集 stdout |
| 观测平台 | 可选 Compose overlay、Docker 轮转、受限 socket 代理、Alloy、Loki、Grafana、Prometheus、链路探针、仪表盘和 11 条告警规则 |
| 文档 | OpenAPI 请求 ID 契约、部署说明、八类故障 Runbook、316 条静态 SQL 指纹代码位置清单及生成脚本 |

核心事件：`http.request.completed` / `http.request.failed`、`db.query.slow`、`dependency.attempt.completed`、`job.started` / `job.completed` / `job.failed` / `job.panic`，以及业务模块的受控成功事件。请求/业务/任务 ID 为 JSON 字符串；HTTP 指标只统计完成事件。

指纹清单不包含 SQL 文本或参数；动态拼接 SQL 可能没有静态清单项，可用日志 operation/caller 定位。pgx 计时以驱动查询生命周期为口径，不声称独立测量所有 Scan/rows.Err/Commit/Rollback 阶段；这些错误仍由 HTTP/任务边界安全记录。更细阶段计时需后续扩展。

## 3. 已执行验证

| 检查 | 结果与范围 |
| --- | --- |
| `go test ./...` | 通过，包括日志配置/脱敏/1000 条无采样/大小限制、HTTP、认证、任务、微信桩和业务测试 |
| 独立库：`FIXPRO_INTEGRATION=1 go test ./... -count=1` | 全量通过；DB_DSN 显式指向本次独立库；包括数据库关联、审计故障、客户隔离、订单/工单/媒体集成测试 |
| `go test -race ./internal/...` | 通过；覆盖诊断状态与并发请求；本轮 race 未开启数据库故障注入 |
| `go vet ./...` / `go build ./cmd/...` | 通过 |
| 根目录 `npm run check` | 后台 lint/构建、客户小程序及师傅小程序类型检查通过 |
| `node scripts/test-log-diagnostics.mjs` | 10 个实际 TS 封装响应桩场景通过：浏览器 JSON/HTML/网络/Blob，小程序 request/upload/download/登录；包含任务响应头事件和安全复制字段 |
| `python3 -m unittest discover -s deploy/observability/probe -p 'test_*.py'` | 2 个测试通过：请求到日志/任务存活、API 失败与日志缺失的区分；不是 Loki 实际查询验收 |
| Ruby YAML 解析 | 7 个 YAML 文件通过语法解析；不是 Docker Compose 合并、Alloy 语义或 Grafana provisioning 验证 |
| `python3 scripts/generate-diagnostic-queries.py` | 生成 316 条代码位置记录 |
| `git diff --check` | 通过 |

迁移验证：独立库先执行至 016，写入旧历史哨兵，然后执行 017；最终版本 17、dirty=false，旧记录 request_id/job_run_id 均保持 NULL。另验证事务 ID 自动写入新历史、同一连接池后续无 ID 事务不串用先前 ID。

审计故障验证：仅独立库安装针对测试资源的拒绝写审计触发器，密码重置失败时密码修改回滚；移除故障后成功审计携带 request_id。Commit 失败的受控驱动测试确认不输出成功事件。

真实 HTTP 冒烟：启动编译后的 Go API 连接独立 PostgreSQL，使用本地隔离监听端口；public ping=200、未匹配路径=404、OPTIONS=204、匿名管理员请求=401、方法不匹配=405。五个请求各有一个完成事件和响应 ID，CORS 暴露 X-Request-Id；非法 ID 重新生成；URL 查询哨兵未出现在 JSON 日志。SIGTERM 后输出 `server.stopped`。这不是代理或容器日志测试。

日志微基准曾运行三轮，约 1.10µs/事件、8 allocations/事件、1267B/事件；该值仅用于了解当时日志底座开销，后续字段排序及限制调整后未重测，不能替代最终业务性能验收。

## 4. AC-01～AC-16 结果

| AC | 状态 | 证据及未完成范围 |
| --- | --- | --- |
| 01 | 本地已验证，生产待验证 | Zap/config 测试、入口构建、真实 JSON 冒烟；生产镜像版本注入未实测 |
| 02 | 本地已验证 | middleware 测试和真实 200/404/405/401/OPTIONS；重复/非法 ID、CORS、状态/流式包装 |
| 03 | 部分已验证 | 独立 PG SQL 错误/慢操作和安全根因；微信 timeout 桩；真实 DB 断连/死锁/池耗尽及 Loki 按 ID 检索待演练 |
| 04 | 本地已验证 | Cause、4xx、未知错误、响应写出前后 panic/真实状态/无追加 JSON 测试 |
| 05 | 本地已验证 | auth 和并发隔离测试、race；真实代理边界未验收 |
| 06 | 本地已验证，平台时间线待验证 | 业务集成、Commit 失败无成功、幂等和关联字段；Grafana 时间线待查询 |
| 07 | 本地已验证 | 017 升级保留旧 NULL；关联事务无串用；审计失败密码回滚、成功关联；生产迁移待部署 |
| 08 | 本地已验证，部署存活待验证 | jobRunId/数量/失败/取消/panic 测试、应用停止；多实例任务及真实告警待部署 |
| 09 | 封装已验证，交互待实测 | 10 个响应桩和类型/构建检查；后台 UI、微信开发者工具/真机点击复制及截图未验收 |
| 10 | 应用边界已验证 | 白名单、PG Detail/错误/panic/对象/查询哨兵；Docker/Loki/通知全链路敏感值扫描待执行 |
| 11 | 待外部验证 | 配置已交付；1000 条事件 ≥99%/60s 集中可见、低基数标签未实测 |
| 12 | 待外部验证 | 有界缓冲/轮转/有限重试已配置；重启、停 Loki 10min、磁盘/丢失重复计数未实测 |
| 13 | 待外部验证 | 30 天生产/7 天测试配置；实际删除、24h 容量、代理 API 权限、访问控制未验收 |
| 14 | 待外部验证 | 仪表盘/规则/独立探针已交付；LogQL、故障/恢复/No Data、真实通知未执行 |
| 15 | 回归已验证，性能未验收 | Go 集成及前端检查通过；缺实施前基线与同条件最终三轮业务压测 |
| 16 | 待外部验证 | Runbook 与代码位置清单可用；5 分钟人工定位、Grafana/真机截图未执行 |

## 5. 平台配置边界与交接

部署步骤、固定镜像标签和风险边界见 [观测栈说明](../../deploy/observability/README.md)。API 不在请求线程向远程 Loki 写日志，不依赖观测组件启动；远程故障的实际可用性和性能仍须压力验证。

上线前必须完成：

1. 有 Docker 的隔离环境合并/运行 Compose，逐组件校验语义和实际镜像存在性/兼容性，登记并锁定 RepoDigests；本次只锁定了标签。
2. 配置私有 Grafana 密码、webhook、环境/实例、通知分组与内部 Runbook 链接。现有规则面向单环境栈；共享 Loki 时须补环境/服务分组，避免跨环境统计。
3. 验证 HAProxy 拒绝 POST、exec、archive 等管理路径；核对受信代理 ID/访问日志策略。观测组件内部日志目前由 Docker 轮转保留，集中白名单默认只覆盖 API 与 migrate；需要采集其他组件时先审核其输出是否可能含 webhook/凭据，再扩展受控采集。
4. 对三类任务按实例验证存活探针；多副本部署须扩展实例集合监测和既有任务调度策略，不能用单实例成功掩盖其他实例异常。
5. 执行 AC-10～14 故障/安全/删除/通知/容量实验；100MiB 本地默认日志预算不保证 24h，Alloy 有限重试耗尽后存在丢失窗口，重启可能重复。
6. 完成最终业务负载压测、真机/页面复制验收、5 分钟排障演练，补充截图和计量结果后，再按 P9 发布顺序执行 staging/生产。

未执行任何生产数据库修改、账号初始化、上线、实际通知或镜像部署。回滚应用时保留 017 的新增可空字段，不默认运行 Down 或删除观测卷。
