# SPEC-012｜Go 后端日志系统与线上问题定位

**状态：** 应用实现及本地验证完成；集中平台与生产验收待完成  
**版本：** V1.0  
**日期：** 2026-10-07  
**适用工程：** `apps/server-go`、`apps/admin-web`、`apps/wechat-mini`、`deploy`、`docs/runbooks`  
**关联文档：** SPEC-002 Go 后端迁移、SPEC-004 订单履约、SPEC-009 后台权限、SPEC-010 师傅登录、SPEC-011 客户登录与数据隔离、技术方案 V1 第 15 节

---

## 1. 目标与核心决策

用户反馈“下单失败”“派单异常”“上传超时”时，运维人员能够凭发生时间、请求 ID 或订单/工单 ID，找到接口访问记录、内部错误根因、代码位置、业务状态变化和部署版本。

1. Go 应用统一采用成熟开源框架 **Uber Zap（`go.uber.org/zap`）**，使用类型化字段和单行 JSON。新增 `internal/platform/logging`，统一初始化、上下文关联、字段约束和敏感信息处理。
2. 生产默认链路为 **应用 stdout → Docker 日志 → Grafana Alloy → Loki → Grafana**。应用不直接向远程日志平台发送网络请求，采集故障不影响正常业务调用。
3. 首期必须完成请求关联、5xx 根因日志、关键业务事件、后台任务诊断、前端错误参考号、集中检索、日志保留和告警；只更换日志框架不视为交付完成。
4. 保持现有 `requestId` API 契约及业务错误码。日志字段沿用 camelCase；时间统一 UTC，Grafana 可按浏览器时区显示。
5. 请求日志与错误根因日志分开：每个请求一条完成记录，每次内部故障由明确的责任层记录一次根因，两者使用相同 `requestId`。
6. 业务操作审计继续由 PostgreSQL 保存；集中诊断日志不替代数据库审计和业务历史。
7. 首期采用请求 ID 完成单体内关联；OpenTelemetry 和真实 `traceId/spanId` 在第二阶段接入，不把 requestId 伪装成 traceId。

框架能力依据 Zap 官方文档：结构化字段、Caller、Stacktrace、Sync、Sampling。此处的架构及默认值属于本项目决策，不是框架默认行为。[Zap 官方文档](https://pkg.go.dev/go.uber.org/zap)

## 2. 当前基线与已确认缺口

本节依据 2026-10-07 工作区代码，实施时应再次核对已有修改。

| 位置 | 已有能力 | 缺口 |
| --- | --- | --- |
| `cmd/server/main.go` | slog JSON 输出 stdout，启动失败日志 | 无统一版本/实例字段、日志配置和 Zap 初始化 |
| `platform/httpx/middleware.go` | requestId、方法、路径、状态码、耗时、panic 堆栈 | 客户端 ID 未校验；OPTIONS 在日志逻辑前返回；缺少身份、路由模板和错误码 |
| `platform/httpx/response.go` | 统一响应，未知错误隐藏为 INTERNAL_ERROR，Error 有 Cause 字段 | 未知错误及 Cause 未统一记录，500 可能缺失根因 |
| `internal/app/app.go` | 自动验收、自动收回和头像清理失败日志 | 缺少 jobRunId、运行耗时/处理数量；头像清理日志缺少原因 |
| `internal/admin/handler.go` | audit_log 查询和部分操作写审计 | writeAudit 未写 request_id，忽略写入失败 |
| 订单与履约模块 | 订单/工单状态及派单历史 | 尚未统一输出可按业务 ID 检索的诊断事件 |
| 后台 `api/http.ts` | 普通 API 错误保留 requestId | 网络、非 JSON、媒体失败尚未统一诊断信息 |
| 小程序 `services/request.ts` | 普通请求发送 X-Request-Id | ApiError 丢失响应 requestId；上传/下载没有统一请求 ID |
| `deploy/compose.yaml` | 本地容器部署 | 无集中采集、日志轮转、保留和告警配置 |

本规格不推断线上已经部署了哪些日志服务；上线验收必须检查实际环境。

## 3. 范围与交付阶段

| 阶段 | 交付内容 | 退出条件 |
| --- | --- | --- |
| A：应用诊断 | Zap、字段规范、请求/错误/业务/任务日志、前端参考号、审计关联 | 注入故障后能从请求 ID 找到安全处理后的原因和代码位置 |
| B：生产可用，首期必做 | Alloy/Loki/Grafana、轮转保留、查询模板、仪表盘、告警、Runbook | 从客户端失败到 Grafana 查询和告警闭环通过验收 |
| C：后续追踪 | OTel HTTP/外部依赖 Span、traceId/spanId、追踪存储与日志跳转 | 独立规格评估成本和部署方式后实施 |

首期不拆微服务、不引入 Kubernetes/ELK，不自建后台日志搜索页面，不记录全量 SQL、请求体、响应体或客户端操作录像。本规格只编写需求，不表示已经安装框架或部署平台。

## 4. 框架与工程约束

### 4.1 Zap 选择与替换

选择 Zap 的理由是官方提供类型化结构字段、调用位置、堆栈、日志级别和可组合 Core，适合本项目统一规范；不是因为现有 slog 无法完成结构化日志。

- 应用使用 `*zap.Logger`，业务代码优先类型化 Field，禁止格式化字符串承载可检索业务字段。
- `logging.New(config)` 返回根 Logger 和清理函数；通过依赖注入传入 HTTP、任务和启动命令。避免模块私自创建 Logger 或依赖全局 Logger。
- `logging.FromContext(ctx)` 自动带上本次请求/任务字段；未绑定上下文时使用已注入的根 Logger，不静默丢弃日志。
- 首期迁移所有项目自有的 slog 调用，包括 migrate、bootstrap-admin、backfill-worker-auth、init-customer-auth 等命令；适配标准库 `http.Server.ErrorLog`，确保服务器级错误进入同一输出规范。
- 日志包装设置正确的 Caller skip，代码位置必须落在实际调用者；有安全诊断字段的错误另记录 `operation`，不要依赖错误响应层的位置定位 SQL 操作。
- 生产 JSON、本地可选 console；生产强制 JSON，禁用颜色。业务日志全部 stdout，日志器自身写入故障使用受控 stderr。
- 正常优雅退出调用 Sync；启动失败先完成清理再退出。业务路径禁止 Fatal/Panic/DPanic 控制流程。
- Zap 的生产预设包含采样；本项目必须显式关闭默认采样，首期请求、根因、关键业务和任务日志不采样，避免高频错误被丢弃。不得以日志条数代表可靠业务审计。[Zap Sampling 说明](https://pkg.go.dev/go.uber.org/zap#hdr-Overview)
- 实施时固定已核验兼容 Go 1.26 的稳定 Zap 版本及镜像版本/摘要，提交 go.mod/go.sum；不得使用浮动 latest。

### 4.2 配置约定

| 配置 | 默认/规则 |
| --- | --- |
| `LOG_LEVEL` | info；生产仅允许 debug/info/warn/error，debug 必须按变更流程短期启用 |
| `LOG_FORMAT` | json；console 仅 local/test 可用 |
| `LOG_SERVICE` | fix-pro-api；工具命令使用各自固定服务名 |
| `APP_VERSION`、`GIT_COMMIT` | 构建注入，生产不得空值或 unknown |
| `LOG_INSTANCE_ID` | 由部署注入容器/主机实例标识，无秘密信息 |
| `HTTP_SLOW_THRESHOLD` | 1s；上传/下载默认 5s，按路由类别覆盖 |
| `DB_SLOW_THRESHOLD` | 200ms |
| `UPSTREAM_SLOW_THRESHOLD` | 2s |
| `JOB_SLOW_THRESHOLD` | 30s |
| `LOG_MAX_EVENT_BYTES` | 16KiB，截断自由文本并设置 truncated=true，保留关联字段和必要堆栈 |

阈值必须为正值；无效级别、格式和阈值使启动失败并给出安全诊断。级别调整采用配置重启，首期不开放公网动态调整接口。缺省生产 info；设为 warn/error 会隐藏正常请求与成功事件，应明确提示其诊断能力下降。

## 5. 日志字段与级别

### 5.1 字段契约

| 字段 | 类型与要求 |
| --- | --- |
| timestamp | UTC RFC3339，含毫秒或更高精度 |
| level、message | 小写级别；简洁固定描述，不插入用户自由文本 |
| service、env、version、gitCommit、instanceId | 每条日志必有；env 使用 local/test/staging/production |
| module、event | 固定枚举；如 order / order.created、http / http.request.completed |
| requestId | 请求链必有，任务无请求时省略 |
| principalType、principalId、orgId | 认证成功后来自服务端；ID 编码为字符串，匿名仅记 principalType=ANONYMOUS |
| method、route、path、status、durationMs | 完成日志必有；route 使用 ServeMux 路由模板，未匹配为 UNMATCHED；path 不含 query，动态敏感段脱敏 |
| errorCode | 统一响应业务码；正常为 OK，非 JSON 流式错误使用受控 HTTP/业务码 |
| orderId、workOrderId、skuId、mediaId | 已知且通过资源访问校验时记录内部 ID，类型为字符串 |
| errorType、errorMessage、operation、caller、stack | 内部错误诊断；errorMessage 必须由安全转换器产生，stack 按第 7 节处理 |
| dependency、upstreamCode、queryName、sqlState | 外部依赖和数据库诊断的受控字段 |
| jobName、jobRunId、result、processedCount、failedCount | 后台任务日志；jobRunId 每轮生成 |
| traceId、spanId | 仅有真实 OTel Span 时写入，首期不存在时省略 |

保留字段禁止业务层覆盖，禁止同一个 JSON 对象产生重复 key。queryName、operation、event 使用代码中定义的稳定名称，不使用 SQL 文本或用户输入。上游状态必须经过白名单/类型校验后记录。

### 5.2 级别与事件责任

- DEBUG：安全的内部调试，不输出凭据或全量对象；默认关闭。
- INFO：正常请求完成、已提交关键业务事件、任务开始/完成、服务启停。
- WARN：普通 4xx 请求完成、权限拒绝、慢操作、可恢复依赖异常；预期不存在/冲突只记录受控业务码，不打印堆栈。
- ERROR：5xx 请求完成、内部依赖失败、panic、任务失败、审计写入失败、日志采集链异常。

请求完成根据最终状态和耗时选择级别，慢且非 5xx 至少 WARN；http.request.completed 的 ERROR 是结果记录，http.request.failed 是根因记录，两者属于不同事件，不视为重复。禁止每层都重复输出同一异常。

## 6. 请求关联与身份上下文

1. 保留 `X-Request-Id` 与 JSON `requestId`。接收单一头值，校验为 `^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`；空值、多个值或非法值均替换为服务端随机 128 位小写十六进制 ID，不回显非法内容。
2. requestId 仅用于诊断关联，不能作为身份或幂等依据。客户端可重复使用 ID，检索必须同时使用环境、时间和路由，不能假设外部 ID 全局唯一。
3. 在认证前创建请求诊断状态并置入 context；认证通过后更新其中的服务端身份快照。由于子 handler 的 WithContext 不会自动更新外层 context，外层完成日志必须从该请求状态读取身份，状态不得跨请求共享，并通过 race 测试。
4. 状态存储最终业务错误码、已记录根因标记和受控资源 ID；一切资源 ID 必须通过授权后绑定，拒绝请求不泄露他人资源信息。
5. 所有响应，包括 OPTIONS、404、405、401、403、下载及上传，返回 X-Request-Id；CORS 增加 `Access-Control-Expose-Headers: X-Request-Id`，保留既有 Origin 白名单。
6. 完成日志必须覆盖 OPTIONS 和路由拒绝。状态包装只认最终响应头，忽略非最终 1xx，不能把后续重复 WriteHeader 当作真实状态；未显式 WriteHeader 的 Write 默认为 200。正确保留 Flusher、Hijacker 及 Unwrap/ResponseController 语义，避免破坏媒体和流式响应。
7. 写出响应后发生 panic 时记录 `responseCommitted=true` 与真实已发送状态，不追加第二份 JSON；写出前才返回统一 500。已断连时记录受控连接/取消结果，不承诺客户端收到参考号。
8. 代理访问日志保留服务端响应 requestId、上游状态、耗时和目标实例；如请求未到 Go，则以代理生成的 ID 和时间定位代理故障。客户端提供的 ID 与代理 ID 可分字段保存，不盲信转发头；客户端 IP 只从受信代理链获取。

## 7. 错误根因、数据库与外部依赖

### 7.1 统一根因捕获

- `httpx.Failure` 接收原始错误，未知错误映射 500 时必须先记录安全根因；已知内部错误使用 `Cause/Unwrap` 保留错误链，不将 err 直接替换成无原因的 INTERNAL_ERROR。
- Service/Repository 通过受控诊断错误携带 operation、queryName、dependency 等白名单字段；Handler 传递原始包装错误，响应层作为 HTTP 根因唯一记录点。已由独立依赖事件记录的故障，以状态标记/明确责任约定避免第二次根因日志。
- 一条根因记录至少包含 requestId、errorCode、errorType、operation、caller，以及安全 errorMessage；意外 5xx 自动附带发生日志时的堆栈，必须明确这不等于错误产生时的堆栈。发生位置通过 operation 和必要的原点堆栈补足。
- 禁止直接 `zap.Error(err)` 输出未经审核的数据库、HTTP SDK 或用户拼接错误。对 pgx 错误提取 SQLSTATE、受控约束别名和 queryName；不输出 Detail、完整 SQL、参数、数据库 DSN、用户数据。
- 上游错误记录 dependency、受控操作名、HTTP 状态、上游错误码及耗时；URL 仅允许主机/路由模板，不含查询串、签名或原始响应体。网络错误只记录类别（timeout、dns、connection 等）和安全摘要。
- panic 值可能包含秘密，禁止直接序列化；记录值类型、安全摘要和栈帧，panic 字符串经安全策略转换，禁止包含任意变量值。HTTP 和后台任务入口均恢复并记录；任务失败继续下一轮，不吞成成功。
- 同一 context 取消/超时不能一律改业务错误码；沿用 API 契约，并在日志区分 client_cancelled、deadline_exceeded 和 dependency_timeout。

### 7.2 数据库诊断

- 首期覆盖下单、派单、验收、登录、权限变更、媒体关联和健康检查的数据库操作，记录失败与慢操作；不全量打印每条成功 SQL。
- 使用统一包装/计时 helper，支持 Query/Exec/Tx；慢查询事件记录 queryName、operation、durationMs、requestId 和允许的业务 ID。
- 对 Query/Rows 生命周期定义清楚：执行耗时与扫描耗时区分，检查 Scan/rows.Err/Commit/Rollback 错误，不把成功返回 Rows 当成整个操作成功。
- 每分钟记录 db.pool.snapshot：open、inUse、idle、waitCount 与 waitDurationMs 累计值及区间增量；不得重复将累计值当区间统计。
- 慢查询、死锁和连接耗尽 Runbook 包含 pg_stat_activity/锁等待/连接池检查；查询只读、限制范围，不默认输出手机号、地址或 SQL 参数。

### 7.3 微信、媒体与后台任务

- 微信手机号兑换、code2Session 等记录操作名、依赖状态、耗时、重试次数；禁止记录 Code、AppSecret、access_token、session_key、OpenID/UnionID 原值及完整手机号。
- 上传、下载、头像清理记录 mediaId（授权后）、大小、受控 MIME/结果、耗时；禁止签名 URL、本地绝对路径、原始文件名和内容。
- 自动验收、自动收回、头像清理每轮有 job.started 与 job.completed/job.failed，共享 jobRunId，记录开始/结束、耗时、处理/失败数量；0 处理也保留完成记录用于存活检查。
- SYSTEM 操作携带 jobRunId；任务中的逐项事件带 orderId/workOrderId/mediaId。定时任务遵循应用生命周期、有限超时和停止信号，退出不得留下无限循环 goroutine。

## 8. 业务事件与数据库审计

### 8.1 必须覆盖的首期事件

| 模块 | 事件/场景 | 必要关联 |
| --- | --- | --- |
| order/cart | 下单成功/失败、幂等命中、幂等冲突 | orgId、principalId、orderId（存在时）、受控幂等结果 |
| fulfillment | 确认/拒单、派单/改派/收回、开始服务、完工提交、审核、客户验收/自动验收 | orderId、workOrderId、fromStatus、toStatus、操作者或 jobRunId |
| auth/authorization | 客户/师傅/后台登录结果、权限拒绝、密码重置、角色变更 | principalType、principalId（已确定时）、orgId、受控结果码 |
| catalog | SKU 发布/下架 | skuId、orgId、principalId、版本 |
| media | 上传完成/失败、访问拒绝、清理结果 | 合法内部 mediaId、大小、耗时、结果 |

状态变更成功事件在事务 Commit 成功后输出；回滚不输出成功。一次批量操作记录汇总，必要逐项记录使用不同 event；诊断日志可能丢失，数据库仍为业务事实来源。幂等键不记录原值，仅记服务端安全摘要或是否命中。

### 8.2 审计关联与失败处理

- 复用已有 audit_log.request_id，所有本期触及的审计写入必须填入 `httpx.RequestID(ctx)`，覆盖角色/用户变更、密码重置等高风险操作。
- 去掉 writeAudit 对错误的忽略。高风险操作的业务更新和审计写入使用同一事务，审计失败则回滚并返回受控错误；已有其它审计场景必须显式声明一致性策略，禁止静默成功。
- 若现有工单/订单历史无关联列，使用增量迁移补充可空 request_id、job_run_id，按实际排查查询加索引；历史数据保留 NULL，不伪造旧 ID。迁移编号以实施时最新序列为准。
- 审计表不写诊断堆栈和秘密信息，业务审计保留遵循已有规则，不套用 Loki 30 天保留策略。

## 9. 前端错误参考号

- 后台和小程序的 ApiError 统一保留 code、requestId、HTTP status；成功响应处理不改变。
- 普通请求、上传、下载均在发起前生成并发送合法 requestId；收到响应优先使用响应 JSON ID，再响应头 ID，缺失时保留客户端 ID 并标记 clientGenerated=true。
- 网络失败、代理返回 HTML、空响应和 JSON 解析失败不得覆盖为二次解析异常；保留状态、失败类别和客户端参考 ID。这类 ID 不代表服务端收到请求，反馈面板必须明确“客户端参考号”。
- 普通错误提示保持简短；下单/派单/登录/上传等失败支持展开“错误详情”并复制参考号、时间、操作名、客户端版本、受控错误码；无凭据、手机号、表单、完整 URL 和用户资料。
- 不增加公开读取日志接口，不向用户展示 SQL、内部错误或堆栈；不自动上报原始敏感信息。首期反馈信息由用户主动复制提供。
- 对后台 JSON/Blob/FormData 和小程序 wx.request/wx.uploadFile/wx.downloadFile 分别验收，避免只覆盖普通 JSON。

## 10. 集中采集、保留与访问

### 10.1 部署要求

- 新增 `deploy/observability`，提供固定版本的 Alloy/Loki/Grafana 配置与 Compose overlay/profile、环境示例、持久卷、健康检查和启动说明；本地默认启动不强制拉起观测栈。
- Alloy 按服务白名单采集 API、维护命令和观测组件日志，解析 Docker 包装及 JSON 时间；保存采集进度，配置有界重试/缓冲，明确重启后的恢复与潜在重复/丢失窗口。禁止承诺端到端 exactly-once。
- Docker socket 或日志目录访问按最小权限部署，socket 不能因只读挂载就视为没有管理能力；优先使用受限代理/授权机制并记录实际权限。
- 本地 Docker `json-file` 轮转基线 max-size=20m、max-file=5；按服务配置并验收生效。此值是磁盘上限基线，不保证时间窗口；生产根据峰值日志速率保证至少 24h 本地缓冲，并验证磁盘容量。
- Loki 首期可使用单实例持久存储，作为诊断设施不得挤占数据库资源；明确单实例故障限制。Grafana 配置持久化、仪表盘/告警以代码 provisioning 或可重放导出交付。
- Loki/Grafana/Alloy 管理端仅内网或受控入口访问；Grafana 禁止匿名、强制认证，凭据由部署密钥注入。Loki API 需网络隔离或认证代理；仅有 Grafana 登录不能保护直接 Loki API。
- API 容器不依赖 Loki 启动/健康，远程日志不可用不得改变业务结果。stdout 阻塞/丢失策略和 Docker 缓冲容量必须明确，进行故障验收并量化实际行为。

Alloy 的 Docker 采集组件作为实现依据。[Alloy Docker 采集文档](https://grafana.com/docs/alloy/latest/reference/components/loki/loki.source.docker/)

### 10.2 标签与保留

- Loki 索引标签只允许低基数字段 service、env、module、level；version/instanceId 默认留 JSON，是否索引需容量评估。
- requestId、principalId、orgId、orderId、workOrderId、jobRunId、path、错误消息禁止做索引标签；通过 JSON 解析过滤或显式配置的 structured metadata 检索。参考官方标签建议。[Loki 标签最佳实践](https://grafana.com/docs/loki/latest/get-started/labels/bp-labels/)
- 生产诊断日志默认保留 30 天，测试 7 天；配置 Loki 实际存储/schema、Compactor 保留和删除机制，不以写入一个环境变量代替验收。磁盘保留时间不是硬容量限制，还需容量监测。[Loki 保留文档](https://grafana.com/docs/loki/latest/operations/storage/retention/)
- 上线报告给出日均/峰值 GiB、索引/压缩实测、30 天预计容量和至少 30% 余量；磁盘使用 >80% 持续 10 分钟告警。
- 首期 Grafana 仅向受信平台运维开放，不向租户管理员开放跨组织日志；若后续开放组织级查询必须实现真正的数据隔离，不能靠用户可编辑的 orgId 过滤条件。

## 11. 快速定位、仪表盘与告警

### 11.1 预置检索

以下 LogQL 假定采集器已按第 10 节配置 service/env 标签，查询前在 Grafana 选择发生时间前后 15 分钟。

```logql
{service="fix-pro-api", env="production"} | json | __error__="" | requestId="REQ_ID"
{service="fix-pro-api", env="production"} | json | __error__="" | orgId="1" | orderId="123"
{service="fix-pro-api", env="production"} | json | __error__="" | orgId="1" | workOrderId="456"
{service="fix-pro-api", env="production"} | json | __error__="" | event="http.request.failed"
{service="fix-pro-api", env="production"} | json | __error__="" | event="http.request.completed" | durationMs > 1000
{service="fix-pro-api", env="production"} | json | __error__="" | jobRunId="JOB_RUN_ID"
```

预置“请求定位”“订单/工单时间线”“错误根因”“依赖与慢请求”“后台任务”“部署版本”查询；Grafana 面板支持 requestId/orderId/workOrderId 跳转。ID 在 URL/查询参数中必须安全编码，不能将用户输入直接拼接为 LogQL。

### 11.2 排障 Runbook

交付 `docs/runbooks/production-logging.md`，包含以下可执行路径：

1. 收集错误参考号、时间、操作和业务 ID，确定 production/staging 及服务器实际时区。
2. 查请求完成事件，确认实际状态/业务码、路由、耗时、身份、实例和版本。
3. 用相同 requestId 查根因、慢查询和依赖事件；凭 queryName/operation/caller 定位代码，结合版本寻找对应部署提交。
4. 状态异常按 orgId + orderId/workOrderId 查事件及数据库历史，核对已提交状态，识别幂等重试、事务失败和后台任务。
5. 未找到请求时检查客户端网络、代理访问日志、服务重启和采集延迟；日志缺失不作为请求未到达的唯一证据。
6. 需要应急本地日志时使用 `docker compose ... logs --since ... server`，按 requestId 固定字符串过滤；避免把整段日志复制到公共渠道。
7. 修复/回滚后重复同一关键业务操作，确认结果与告警恢复，记录关联 ID、变更版本和根因。

必须包含：普通 SQL 错误、连接池耗尽、微信超时、panic、派单状态冲突、任务停跑、采集断流、磁盘不足的检查命令与恢复步骤。

### 11.3 仪表盘与告警

仪表盘展示 HTTP 请求量/5xx/业务错误码/P95、慢路由、根因分布、依赖失败、数据库池快照、任务最近成功时间、实例/版本和采集链状态。HTTP 统计只计算 `http.request.completed`，不混入根因 ERROR 或业务成功事件；剔除健康探测/OPTIONS，区分流式与普通请求。日志解析失败单独监测。

| 告警 | 默认触发条件 | 去重/处置 |
| --- | --- | --- |
| API 异常率 | 5 分钟有效请求 ≥20，5xx 比例 >5%，持续 5 分钟 | env+service；带路由 Top N、查询链接和 Runbook |
| 核心操作不可用 | 下单/派单路由 5 分钟内 ≥3 次内部失败 | env+route；与异常率告警合并避免刷屏 |
| Panic | 任一 http.panic 或 job.panic | env+module+operation；附 requestId/jobRunId、版本 |
| 外部依赖失败 | 单依赖 5 分钟 ≥5 次失败且失败率 >20% | dependency；分母来自实际尝试事件，不能仅靠失败日志计算 |
| 后台任务异常 | 连续 3 次失败，或预期运行实例 5 分钟无任何结束事件 | jobName+instanceId；明确正常停机/部署静默窗口 |
| 慢请求 | 普通业务路由 5 分钟 ≥20 请求且 P95 >1s，持续 10 分钟 | route；媒体使用独立 5s 阈值 |
| 日志链异常 | 采集失败/延迟 >60s 持续 5 分钟，或日志源探测有请求而无采集 | 监控采集器健康/指标；不能只依赖已故障的日志链 |
| 容量不足 | 日志存储磁盘 >80%，持续 10 分钟 | 存储实例；附清理/扩容说明 |

外部依赖成功/失败需各一条 attempt.completed（含 result、耗时），根因另按规则记录，防止分母重复。首期告警由 Grafana 管理，基础设施链路/磁盘健康使用独立的监控数据源或外部探测；交付时固定实际实现，不允许依赖故障日志自报存活。

通知渠道配置由运维提供实际接收人和部署密钥；实施必须做测试通知、故障通知、恢复通知及去重验收。阈值为首期基线，上线一周根据实际负载调整。没有流量与查询失败必须区分，不把 No Data 一律当正常。

## 12. 敏感信息与可靠性

- 禁止输出 Authorization、Cookie/Set-Cookie、密码及哈希、Token、微信 Code/密钥、手机号/完整地址、备注自由文本、身份证、DSN、签名 URL 和媒体内容；禁止序列化整个 Request、Principal、业务 DTO 或第三方响应。
- 根因保留诊断类别、操作位置和受控摘要，而不是以“内部日志”为由输出秘密。敏感键过滤只是兜底，不能代替字段白名单和错误类型转换。
- 所有自由文本限制长度并控制字符安全；JSON 编码负责换行转义。脱敏/截断异常不得 panic 或回退成原文输出，使用 redacted/unknown_error。
- 日志写入失败不得改变业务事务结果；审计写入失败遵循第 8 节事务规则。必须记录日志平台故障时的实际丢失/阻塞边界，日志系统不保证业务审计持久性。
- 生产初始不启用采样；若后续启用，只允许明确的低价值成功事件，并保持 ERROR/关键事件不采样，仪表盘/告警切换到独立计数数据源后才能上线。
- 以固定负载比较原实现和新实现：相同硬件/请求组合/并发/时长，至少三轮；稳态 P95 延迟增幅目标 ≤5%、CPU 增幅目标 ≤10%，超出须定位字段序列化、stdout 或采集压力并提供结果，不以微基准代替业务验证。

## 13. 验收标准

| 编号 | 场景与通过条件 |
| --- | --- |
| AC-01 | API 和维护命令使用统一 Zap 初始化；生产 JSON 字段/类型符合契约，版本和 Caller 可追溯 |
| AC-02 | 合法/非法/重复请求头、OPTIONS/404/405/认证失败均有请求 ID 和一次完成记录，非法值不回显 |
| AC-03 | 注入数据库连接/约束/超时错误，通过 requestId 找到 queryName、SQLSTATE 或安全原因、operation、版本；客户端不出现内部详情 |
| AC-04 | 普通 4xx 无无意义堆栈；未知错误、Cause 链、panic 均有根因；写出响应后 panic 不追加 JSON、不误报已发送状态 |
| AC-05 | 认证后的完成日志具备正确身份，匿名及跨组织拒绝不泄露资源；并发请求通过 race 测试，无字段串用 |
| AC-06 | 下单、派单、完工/验收事件可按组织和业务 ID 还原；Commit 失败无成功事件，幂等重试不会伪造第二次业务成功 |
| AC-07 | 高风险操作与审计同事务且写 request_id；审计失败回滚；历史关联迁移保留旧数据 |
| AC-08 | 三类任务有独立 jobRunId、耗时和结果；注入任务 panic/失败仍能进入后续轮次，优雅退出可停止任务 |
| AC-09 | 后台和小程序 JSON/上传/下载/网络失败/HTML 响应均保留可复制参考号，并区分客户端与服务端 ID |
| AC-10 | 注入包含手机号、Code、密钥、DSN、URL 查询串和 pgx Detail 的错误/请求，应用日志、Docker 日志和 Grafana 均查不到原始敏感值 |
| AC-11 | Alloy/Loki/Grafana 在实际部署环境可检索；低基数标签正确；正常负载下 99% 测试事件 60 秒内可见，测试集合不少于 1000 条 |
| AC-12 | 采集器重启、Loki 暂停 10 分钟、日志轮转和磁盘压力演练通过；API 保持可用，报告恢复时间、重复/丢失数量和缓冲边界 |
| AC-13 | 保留与删除在隔离测试环境使用缩短期限实测，生产配置核对 30 天；磁盘容量报告和访问控制通过验证 |
| AC-14 | 仪表盘及查询按第 11 节运行；故障/恢复/断流/No Data 告警和实际接收渠道通过测试，无敏感通知内容 |
| AC-15 | 基准与回归覆盖关键业务、媒体/流式响应；结果包含硬件、负载、对比数据和代码版本，无未解释的性能回退 |
| AC-16 | 演练者按 Runbook 从有效参考号在 5 分钟内找到根因类别与代码操作位置，按订单/工单 ID 找到业务状态时间线；留下截图与关联日志证据 |

## 14. 实施清单与完成定义

1. 建立 logging 包、配置、字段/安全错误转换及 Zap 依赖；补应用与命令初始化。
2. 改造 httpx 请求状态、Failure/Cause、响应状态包装和 CORS；接入认证身份。
3. 为首期数据库/微信/媒体操作、业务事件、任务及审计补充关联；添加必要增量迁移。
4. 改造后台与小程序错误参考号和 JSON/媒体封装；更新 OpenAPI requestId 说明。
5. 交付观测部署、固定版本清单、仪表盘、告警、实际通知配置、Runbook 和验收报告。

交付证据保存于 `docs/test-reports/PLAN-012-logging-e2e.md`（实施阶段创建），包含版本、命令、部署配置、故障注入、安全/性能/保留结果和截图。阶段 A+B 及 AC-01～AC-16 全部完成后才能将本 SPEC 改为“已实现并验收”；本文件创建不代表实现完成。本次实施结果及未验收范围见 [PLAN-012 验收记录](../test-reports/PLAN-012-logging-e2e.md)。

## 15. 官方参考与后续兼容

- [Zap 官方仓库](https://github.com/uber-go/zap)：框架及维护状态。
- [Zap API 文档](https://pkg.go.dev/go.uber.org/zap)：类型化字段、Caller、Stacktrace、Sync 和 Sampling。
- [Alloy Docker 日志采集](https://grafana.com/docs/alloy/latest/reference/components/loki/loki.source.docker/)：容器日志与采集组件。
- [Loki 标签最佳实践](https://grafana.com/docs/loki/latest/get-started/labels/bp-labels/)：索引基数边界。
- [Loki 日志保留](https://grafana.com/docs/loki/latest/operations/storage/retention/)：Compactor 与删除配置。
- [OpenTelemetry 日志数据模型](https://opentelemetry.io/docs/specs/otel/logs/data-model/)：后续真实 trace/span 与日志关联；字段映射由阶段 C 明确。

本规格对 Go 日志选型以 Zap 替代 SPEC-002 中的 slog 技术约定；保持已有隐私、权限、API 和业务审计约束。技术方案中 JVM 指标等旧 Java 表述不适用于当前 Go 实现，后续可观测性扩展以实际 Go 运行时和 PostgreSQL 为准。
