# 懂修匠 日志与健康观测栈

链路：Go Zap JSON → Docker json-file → 受限 Docker 日志代理 → Alloy → Loki → Grafana。Prometheus 和 log-probe 独立监测日志链，不需要应用内远程日志写入。

## 启动与配置

在仓库根目录执行，先创建私有环境文件：

```bash
cp deploy/observability/.env.example deploy/observability/.env.local
# 编辑 .env.local：设置 Grafana 密码、实际 webhook URL 和环境。
docker compose --env-file deploy/observability/.env.local \
  -f deploy/compose.yaml -f deploy/observability/compose.observability.yaml config --quiet
docker compose --env-file deploy/observability/.env.local \
  -f deploy/compose.yaml -f deploy/observability/compose.observability.yaml up -d --build
```

打开 `http://localhost:3000`，登录后进入 懂修匠 文件夹。Loki、Docker 代理、Alloy、Prometheus 不发布主机端口。跨主机入口需配置受控 TLS 代理，不能直接发布 Loki API。当前 overlay 针对已有本地 Compose，不是已验证的生产部署方案；生产数据库、媒体适配器和密钥遵循原有部署要求。

维护命令容器需添加 `fixpro.logs=true` 标签、保留结束后的容器直到采集确认，并使用同一轮转策略。初始化工具 stdout 保留计数报告，省略媒体 object-key 清单；完整清理清单仍由原有数据库清单管理。

## 固定版本

Zap 1.28.0、httpsnoop 1.0.4；Alloy 1.11.0、Loki 3.6.4、Grafana 12.2.3、Prometheus 3.7.0、node-exporter 1.9.1、HAProxy 3.2.0-alpine、probe Python 3.13.7-alpine。全部使用明确标签。生产前按目标平台拉取并登记 RepoDigests，完成漏洞与兼容核验后锁定镜像摘要；本机无 Docker，目前未完成镜像运行验证。

配置按官方 [Alloy Docker source](https://grafana.com/docs/alloy/latest/reference/components/loki/loki.source.docker/)、[Loki write](https://grafana.com/docs/alloy/latest/reference/components/loki/loki.write/) 和 [Loki retention](https://grafana.com/docs/loki/latest/operations/storage/retention/) 设计。未启用实验性 Alloy WAL，使用持久化读取进度、Docker 本地日志和有界发送重试；发送重试耗尽/读取进度已推进时可能丢失，重启亦可能重复。必须以演练量化实际窗口，不能宣称无损或 exactly-once。

## 容量与权限

本地默认每服务 20MiB × 5 个文件，非阻塞日志缓冲 4MiB。缓冲耗尽可能丢弃日志，不阻塞业务；100MiB 本地上限不保证 24 小时。生产设置 `DOCKER_LOG_MAX_SIZE` 与 `DOCKER_LOG_MAX_FILE`，按 `峰值日志字节/秒 × 86400 × 1.3` 测算至少 24h 本地预算，另根据实测压缩比例计算 30 天 Loki 容量。

Loki Compactor 开启保留删除，默认 720h；测试设置 168h。保留不是磁盘硬上限。缩短期限测试需独立栈/卷、遵循 Loki 最小保留周期与索引周期，不删除共享卷验证保留。磁盘监测以 Linux 主机为准，macOS Docker Desktop 的 node-exporter 不代表宿主真实磁盘，需要额外宿主监测。

只有 HAProxy 容器挂载 Docker socket；它限制 GET/HEAD 与容器枚举/日志/版本路径，Alloy 不直接访问 socket。只读挂载本身仍可调用管理 API，代理容器必须作为受信基础设施运行。测试 POST、exec、读取归档等路径确实拒绝后再上线。Grafana 密码/webhook 仅由私有环境注入，禁止提交。

## 运行验收

1. API 与 `/actuator/health` 可用，Alloy `/metrics`、Loki `/ready`、Grafana 数据源和 provisioning 无配置错误。
2. 请求 ID 在客户端/Go/Loki 一致，1000 条事件 ≥99% 在 60 秒内可查。
3. `log-probe` 产生带 ID 的公共 ping，请求后查询 Loki；exported metrics 区分 api_up、loki_up、delivery_up、jobs_up，Prometheus 采集后由 Grafana 告警。
4. Loki 暂停 10 分钟、Alloy 重启、日志轮转和隔离磁盘压力，API 维持可用，记录丢失/重复和恢复时间。
5. 真实告警接收人验证测试、故障、恢复通知。No Data 与查询错误分别告警。调整环境过滤、静默窗口和 runbook_url 为实际内部文档链接。
6. 图表与规则目前为代码交付，尚待真实 Loki/Grafana 校验 LogQL 和告警执行；日志数据不是可靠审计计数。

完整处置见 `docs/runbooks/production-logging.md`。关闭观测服务不会改变 API 启动依赖，保留本地轮转和磁盘监测；不要运行 `down -v` 作为普通回滚。

管理员初始化的自动生成密码只写入交互终端，或显式 `-password-output` 创建的 0600 新文件；自动化也可注入 `BOOTSTRAP_ADMIN_PASSWORD`，不在容器 stdout 打印初始密码。
