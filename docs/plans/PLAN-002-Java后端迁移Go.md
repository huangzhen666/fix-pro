# PLAN-002｜FixPro Java 后端迁移 Go 实施计划

**状态：** Implemented；真实 MySQL/Compose E2E 待在具备 Docker 的环境执行  
**版本：** V1.0  
**日期：** 2026-08-04  
**对应 Spec：** `docs/specs/SPEC-002-Java后端迁移Go.md` V1.0  
**关联验收：** `docs/specs/SPEC-001-SKU购物车下单正向链路.md` V1.3  
**迁移方式：** 契约冻结 → Go 并行实现 → 独立数据库验收 → 切流 → 删除 Java

---

## 1. 交付目标

将当前 Java Spring Boot 后端替换为 Go 模块化单体，同时保证 React 管理后台和微信小程序不因语言迁移修改 API 适配逻辑。

最终交付状态：

```text
React 管理后台 ─┐
                ├─ /api/v1 → Go API → MySQL / Redis / Object Storage
微信小程序 ─────┘

Java / Maven / Spring Boot / Flyway
→ 从生产构建、Compose、CI 和主工程删除
→ 历史仅通过 Git 保留
```

Go 服务必须完整承接：

- 分类新增、编辑、排序、启停；
- SKU 图片、草稿、发布版本和下架；
- 小程序分类目录、基础搜索和服务详情；
- 服务端购物车、故障描述和故障图片/视频；
- 幂等下单、订单快照和后台订单查询；
- Basic Auth、本地客户 Token、统一响应、错误码、requestId 和媒体权限。

## 2. 当前工程基线

| 范围 | 当前状态 | 迁移处理 |
|---|---|---|
| Java 后端 | `apps/server`，Spring Boot 3.5、Java 21、Flyway、MyBatis/JdbcTemplate，功能正在实现 | 功能冻结，仅作为契约和行为参考 |
| 数据库 | `V1__baseline.sql`、`V2__sku_cart_order_slice.sql` | 转换为 golang-migrate 文件，默认重建本地/测试库 |
| React | 分类、SKU、订单页面已接 `/api/v1` | 不重写，只做 Go 契约回归 |
| 微信小程序 | 首页、全部服务、我的、搜索、详情、购物车、下单 | 不重写，只做 Go 契约回归 |
| 基础设施 | MySQL 8.4、Redis、MinIO Compose | 继续复用，迁移期为 Go 使用独立数据库 |
| 验证环境 | 前端 `npm run check` 已可执行；Java 验证受本机环境影响 | Go 建立独立、可重复的测试与 Docker 构建链路 |

## 3. 实施原则

1. 不逐行翻译 Java Controller；以 SPEC、OpenAPI、数据库约束和验收场景为事实来源。
2. 迁移期新建 `apps/server-go`，不得直接覆盖 `apps/server`。
3. Java 与 Go 使用不同数据库：建议 `fix_pro_java` 与 `fix_pro_go`，禁止双写同库。
4. `/api/v1` 路径、JSON 字段、错误码、金额单位和 ID 字符串格式不变。
5. Go 业务事务以数据库锁和唯一约束为最终保证，不使用进程内锁替代幂等。
6. 数据库只由 golang-migrate 管理；Go 服务不读取或写入 Flyway 历史。
7. 每个里程碑都必须保持 `go test ./...`、`go vet ./...` 和契约测试可运行。
8. 在 Go 完成全部验收前，不删除 Java、不改默认 8080 切流、不修改现有数据库卷。
9. 删除 Java 和旧数据库卷属于最终切流动作，执行前必须备份并再次确认目标。

## 4. 里程碑与关键路径

| 里程碑 | 结果 | 依赖 | 切流阻塞 |
|---|---|---|---:|
| G0 契约冻结 | OpenAPI、错误码和 JSON fixture 固化 | 无 | 是 |
| G1 Go 平台底座 | API 可启动、健康检查、日志、认证和优雅停机 | G0 | 是 |
| G2 数据库接管 | 空库 migration、sqlc 和真实 MySQL 测试通过 | G1 | 是 |
| G3 Media + Catalog | 后台分类/SKU/图片与小程序目录等价 | G2 | 是 |
| G4 Cart | 服务端购物车和故障资料等价 | G3 | 是 |
| G5 Order | 幂等下单和后台查单等价 | G4 | 是 |
| G6 全量验收 | SPEC-001、契约、安全、并发和性能通过 | G5 | 是 |
| G7 切流与收尾 | Go 成为唯一后端，Java 与 Flyway 退出 | G6 | 是 |

关键路径：

```text
G0 → G1 → G2 → G3 → G4 → G5 → G6 → G7
```

Media 与 Catalog Repository 可在 G2 后并行实现，但 SKU 发布依赖 Media 校验完成。

## 5. 工作分解

### G0｜冻结 API 与迁移输入

#### GO-0.1 建立迁移清单

盘点并记录：

- 当前所有 `/api/v1`、`/actuator/health` 路由；
- React `apps/admin-web/src/api` 中使用的请求和类型；
- 小程序 `apps/wechat-mini/miniprogram/services` 中使用的请求和类型；
- `ErrorCode.java` 中全部错误码与 HTTP 状态；
- `V1/V2` 表、列、索引、种子数据和金额/时间语义；
- 媒体大小、类型、数量和访问权限；
- SPEC-001 AC-01～AC-12。

产出：`apps/server-go/api/migration-inventory.md`。

验收：清单中的每个前端 API 都能映射到 SPEC-002 第 7 节接口。

#### GO-0.2 固化 OpenAPI

创建 `apps/server-go/api/openapi.yaml`：

- 定义统一 `ApiResponse`、分页、错误响应；
- ID 全部为 JSON string；
- 金额为 `int64` 分；
- 时间为 UTC ISO-8601；
- 定义 multipart 上传；
- 定义 Basic/Bearer 安全方案；
- 定义当前分类、SKU、媒体、购物车和订单接口；
- 为所有错误响应补示例。

验证：OpenAPI lint 通过；React/小程序现有类型与 Schema 人工核对无冲突。

#### GO-0.3 建立契约 fixture

新增 `apps/server-go/test/contract/testdata`：

```text
catalog-categories.json
catalog-services.json
admin-sku-detail.json
cart.json
order-created.json
admin-order-detail.json
error-category-in-use.json
error-cart-sku-changed.json
```

Fixture 只表达外部契约，不保存 Java 内部类名或数据库 DO。

#### GO-0.4 冻结 Java 功能

- Java 只接受安全或迁移阻塞修复。
- 新业务需求先更新 Go Spec/Plan，不继续扩展 Java。
- 记录 Java 当前可用提交/工作区快照，避免对照行为漂移。

退出条件：G0 产物经过一次人工评审，所有接口有明确 owner 与验收 fixture。

### G1｜Go 工程与平台底座

#### GO-1.1 初始化工程

创建：

```text
apps/server-go/
├─ cmd/api
├─ cmd/migrate
├─ api
├─ db/migrations
├─ db/queries
├─ internal/app
├─ internal/platform
├─ internal/catalog
├─ internal/media
├─ internal/cart
├─ internal/order
├─ test/contract
└─ test/integration
```

初始化要求：

- `go.mod` 指定 Go 1.26；
- 固定 Go 工具和库版本；
- 提交 `go.sum`；
- `.gitignore` 排除二进制、覆盖率、临时媒体和本地环境文件；
- `Makefile` 或 PowerShell 兼容脚本提供统一命令。

#### GO-1.2 配置加载与生产保护

实现 `internal/platform/config`：

- 环境变量映射；
- 必填项校验；
- DB pool 参数；
- HTTP timeout；
- CORS 白名单；
- Media driver；
- local 管理员和客户 Token。

必须测试：

- local 使用明确默认值可启动；
- production 默认密码启动失败；
- production 本地 Token 启动失败；
- production `MEDIA_DRIVER=local` 启动失败；
- DSN 不输出到日志。

#### GO-1.3 HTTP Server

使用 `net/http`：

- `http.ServeMux` 路由；
- `ReadHeaderTimeout、ReadTimeout、WriteTimeout、IdleTimeout`；
- 最大 Header 与 JSON Body 限制；
- 优雅停机，最长 15 秒；
- `GET /actuator/health`；
- `GET /api/v1/public/ping`。

#### GO-1.4 通用中间件

实现顺序：

```text
panic recovery
→ requestId
→ access log
→ CORS
→ body limit
→ authentication
→ route handler
```

统一日志使用 `slog` JSON；禁止记录 Authorization、完整手机号和媒体地址。

#### GO-1.5 统一响应与错误

实现：

- `httpx.WriteSuccess`；
- `httpx.WriteError`；
- 可比较的业务错误码；
- Validation、Not Found、Conflict、Forbidden、Internal 映射；
- 未知错误日志保留 root cause，对客户端隐藏内部信息。

#### GO-1.6 本地认证

- Admin Basic Auth；
- local `Bearer local-customer-1`；
- `Principal{OrgID, SubjectID, Role}` 进入 context；
- 非 local 不注册本地客户 Token；
- Admin 与 Customer 路由分组授权。

退出条件：无需数据库即可运行平台测试，健康/Ping/认证/错误响应契约通过。

### G2｜MySQL、migration 与 sqlc

#### GO-2.1 转换数据库迁移

将 Java Flyway SQL 转为：

```text
000001_baseline.up.sql
000001_baseline.down.sql
000002_sku_cart_order_slice.up.sql
000002_sku_cart_order_slice.down.sql
```

要求：

- Up 结果与 SPEC-001 数据模型一致；
- 保留字符集、排序规则、唯一约束和索引；
- 分类种子和本地客户可重复验证；
- Down 明确为本地开发用途，按依赖逆序删除；
- 不复制 `flyway_schema_history`；
- migration 命令支持 `up、version`，生产不暴露任意 `down` 接口。

#### GO-2.2 数据库连接

实现：

- `database/sql` + MySQL Driver；
- `PingContext` 启动检查；
- 最大/空闲连接和连接生命周期；
- `parseTime=true` 和 UTC；
- 关闭时释放连接；
- 健康接口区分 liveness/readiness。

#### GO-2.3 配置 sqlc

创建 `db/sqlc.yaml` 与查询目录：

- Engine 为 MySQL；
- 生成到 `internal/dbgen`；
- ID、金额和时间类型显式 override；
- 生成代码只由 sqlc 修改；
- CI 重新生成后工作区必须无差异。

#### GO-2.4 Repository/Tx 抽象

- 定义窄接口，不创建通用万能 Repository；
- Service 显式 `BeginTx`；
- sqlc 使用 `WithTx`；
- context 贯穿 Handler → Service → Query；
- 事务错误必须回滚；
- duplicate key、deadlock、lock timeout 有统一识别。

#### GO-2.5 Schema 集成测试

使用 Testcontainers MySQL 8.4：

- 空库 up 成功；
- down/up 本地验证成功；
- 种子数据正确；
- 所有唯一约束可触发；
- JSON、毫秒时间、utf8mb4 和 BIGINT 行为正确；
- `schema_migrations` 为最新版本；
- 不存在 `flyway_schema_history`。

退出条件：任意开发者可以从空库运行 migration，并运行一次真实 MySQL Repository 测试。

### G3｜Media 与 Catalog 等价实现

#### GO-3.1 Media 存储端口

定义：

```go
type ObjectStorage interface {
    Put(ctx context.Context, key string, src io.Reader) error
    Open(ctx context.Context, key string) (io.ReadCloser, error)
    Delete(ctx context.Context, key string) error
}
```

实现 local 文件系统适配器：

- 根目录配置化；
- 路径 normalize 与越界保护；
- 随机对象 Key；
- 测试使用 `t.TempDir()`；
- 不用原始文件名拼路径。

S3/COS/OSS 适配只定义端口与配置，本阶段可不完成生产实现，但 production 必须拒绝 local driver。

#### GO-3.2 Media 上传和读取

实现全部媒体 API：

- multipart 流式读取；
- 图片 10 MB、视频 50 MB；
- 文件签名 + MIME；
- SKU 图片 1 封面 + 最多 8 轮播；
- 故障图片最多 6、视频最多 2、合计最多 8；
- 私有媒体鉴权；
- 公共媒体必须被当前已发布 SKU 引用；
- `nosniff`、公共缓存和私有 `no-store`。

测试：伪扩展名、路径穿越、超限、跨客户、未发布图片、已关联媒体删除。

#### GO-3.3 分类管理

实现：

- 启用/全部分类查询；
- 新增、编辑名称和排序；
- 启停；
- SKU 数统计；
- 已发布 SKU 存在时返回 `CATEGORY_IN_USE`；
- `org_id` 数据范围。

#### GO-3.4 SKU 工作副本

实现：

- Admin SKU 分页/搜索/详情；
- 创建草稿；
- 编辑 + `version` 乐观锁；
- 编码唯一；
- 分类、服务承诺、固定价、单位和图片校验；
- 已发布 SKU 编辑不影响当前公共版本。

#### GO-3.5 SKU 发布与下架

发布事务：

1. 锁定 SKU 或校验乐观版本；
2. 校验启用分类和 `FIXED`；
3. 校验封面/轮播状态、用途和组织；
4. 生成完整、不可变 JSON 快照；
5. 写 `service_sku_version`；
6. 更新当前发布指针；
7. 写审计/Outbox；
8. 提交。

下架不删除版本和历史媒体。

#### GO-3.6 公共目录与搜索

- 公共服务列表只读发布版本；
- 分类接口返回启用分类及所属发布 SKU；
- 基础搜索只匹配发布版本名称/简述；
- 服务详情下架后返回 `SKU_NOT_AVAILABLE`；
- 图片 URL 和轮播顺序与当前契约一致。

退出条件：React 分类/SKU 页面和小程序首页/全部服务/搜索/详情切到 Go 后正常工作，AC-01、AC-02、AC-10～AC-12 通过。

### G4｜Cart 与故障资料

#### GO-4.1 购物车 Repository

- 按 `(org_id, customer_id)` 唯一；
- 首次读取可返回空视图，不强制创建；
- 首次加购原子创建购物车；
- 购物车项唯一约束处理重复加购；
- 查询同时返回 SKU 发布快照和故障媒体。

#### GO-4.2 加购与数量

- 只允许当前 `PUBLISHED` SKU；
- 锁定当前版本和单价；
- 数量 1～99；
- 重复加购累计，超过 99 返回校验错误，不静默截断；
- 小计和总计用 `int64`，检查乘法/加法溢出；
- 修改/删除校验客户归属。

#### GO-4.3 故障资料

- 描述 trim 后 5～500 字；
- 媒体必须属于当前客户、用途正确且 READY；
- 覆盖保存媒体 ID 列表；
- 校验图片/视频数量；
- 删除购物车项只解除临时关联；
- 已产生订单的媒体不删除。

退出条件：小程序购物车、数量、上传、资料保存和重新进入恢复正常，AC-03、AC-08、AC-09 通过。

### G5｜Order 与后台查单

#### GO-5.1 幂等组件

- 请求体规范化后计算 SHA-256；
- 唯一键 `(org_id, principal_id, idempotency_key)`；
- 相同 Key/相同 Hash 返回首次结果；
- 相同 Key/不同 Hash 返回 `ORDER_SUBMIT_DUPLICATED`；
- 并发请求依赖数据库唯一约束，不依赖 `sync.Mutex`；
- 处理中状态和失败清理语义明确。

#### GO-5.2 创建订单事务

事务顺序：

1. 获取或创建幂等占位；
2. `SELECT ... FOR UPDATE` 锁定客户购物车；
3. 校验非空；
4. 校验每项故障描述和 READY 媒体；
5. 校验 SKU 仍发布、版本/价格不变；
6. 服务端重算小计/总计；
7. 插入订单、订单项和媒体关联；
8. 固化 SKU、服务承诺和故障资料快照；
9. 清空购物车项和临时关联；
10. 保存幂等响应；
11. 提交。

任一步失败必须保留购物车和故障资料。

#### GO-5.3 订单号与金额

- 订单号服务端生成并有唯一约束；
- 金额为整数分；
- 不信任客户端总额、SKU 名称、价格或客户 ID；
- 调价返回 `CART_SKU_CHANGED`；
- 状态为 `WAITING_PAYMENT`，支付状态语义保持当前范围。

#### GO-5.4 Admin 订单查询

- 分页、订单号搜索、创建时间倒序；
- 列表手机号脱敏；
- 详情管理员可读取完整手机号；
- 展示服务承诺、故障描述和媒体快照；
- 所有查询附带组织范围。

退出条件：小程序提交订单、结果页和 React 订单中心在 Go 上通过 AC-04～AC-07。

### G6｜契约、并发、安全与端到端验收

#### GO-6.1 静态检查

```powershell
go fmt ./...
go vet ./...
go test ./...
go test -race ./...
```

CI 使用“格式检查”而不是自动修改后提交。

#### GO-6.2 契约回归

- 用 OpenAPI 和 fixture 校验 Go 响应；
- React、小程序 API 类型无需修改；
- 所有错误码与 HTTP 状态一致；
- 空数组、null、分页和时间格式一致；
- ID 大于 JavaScript 安全整数时仍以字符串返回。

#### GO-6.3 MySQL 集成与并发

- migration 空库；
- 乐观锁冲突；
- 两次并发发布；
- 两次并发加购；
- 相同幂等键并发下单；
- 不同幂等键并发下单；
- deadlock/lock timeout 有限处理；
- 事务失败不产生半张订单。

#### GO-6.4 媒体安全

- 伪 MIME、脚本、SVG、双扩展名拒绝；
- 图片/视频大小和数量边界；
- 50 MB 视频流式上传内存验证；
- 客户 B 读取客户 A 媒体为 403；
- 下架 SKU 图片不再公共读取，历史订单管理员仍可受控读取。

#### GO-6.5 性能与稳定性

- 100 SKU 目录 p95 < 200 ms；
- 普通 API p95 < 300 ms；
- `go test -race` 无竞争；
- 优雅停机不接受新请求，在 15 秒内退出；
- DB pool 不泄漏；
- context 取消能终止 SQL 和文件读取。

#### GO-6.6 SPEC-001 全量验收

使用 Go + `fix_pro_go` 独立数据库执行 AC-01～AC-12和演示脚本，不接受手工改库绕过后台操作。

退出条件：生成一份带命令、日期、结果和失败截图/日志链接的验收记录。

### G7｜切流、回滚点与 Java 退出

#### GO-7.1 切流前检查

- G0～G6 全部完成；
- 备份目标数据库；
- 确认是否存在需要保留的数据；
- 确认 Go migration version；
- 确认 production 禁用 local Token/default password/local media；
- 确认镜像以非 root 运行；
- 确认前端无需发布新包即可指向相同域名。

#### GO-7.2 Compose/代理切换

迁移期建议：

```text
Java reference: localhost:8080 + fix_pro_java
Go candidate:    localhost:8081 + fix_pro_go
```

切流时：

1. 进入维护窗口；
2. 停止 Java 写入；
3. 完成数据备份/重建或 baseline；
4. 执行 Go migration up；
5. 启动 Go 8080；
6. 健康、Ping、分类、SKU 和订单 smoke test；
7. 恢复流量；
8. 观察错误率、延迟、数据库连接和订单创建。

#### GO-7.3 回滚策略

切流前回滚：直接停止 Go，Java 与原数据库保持不变。

Go 尚未产生新业务写入时：

- 停 Go；
- 恢复代理到 Java；
- 恢复切流前数据库快照。

Go 已产生新业务写入后：

- 禁止直接把流量切回旧 Java 并继续写；
- 优先 Go 前向修复；
- 若必须回滚，进入维护窗口，导出新增数据并制定显式兼容/回灌步骤；
- 不把 migration `down` 当作生产数据回滚方案。

#### GO-7.4 删除 Java

观察窗口和最终验收通过后：

- `apps/server-go` 归位为 `apps/server`；
- 删除 `pom.xml、mvnw*、.mvn、src/main/java` 和 Flyway 运行配置；
- Compose/Dockerfile/CI 改为 Go；
- 根 README、运行手册和技术方案更新；
- 删除便携 JDK、Maven 缓存等临时验证文件；
- 保留 Java Git 历史和迁移验收报告。

## 6. 推荐提交批次

1. `docs(go): freeze api contract and migration inventory`
2. `feat(go): initialize server platform and http middleware`
3. `feat(go): add mysql migrations sqlc and repositories`
4. `feat(go): implement secured media storage`
5. `feat(go): implement category and sku publishing`
6. `feat(go): implement public catalog and search`
7. `feat(go): implement server-side cart and fault evidence`
8. `feat(go): implement idempotent order transaction`
9. `feat(go): add admin order queries and contract tests`
10. `test(go): add mysql concurrency security and e2e coverage`
11. `build(go): switch compose ci and production image`
12. `chore(server): remove java spring and flyway runtime`

每个提交必须可编译、测试通过，不能在同一提交中同时大规模生成代码和修改无关前端样式。

## 7. AC-GO 追踪矩阵

| SPEC-002 验收 | 主要任务 | 证据 |
|---|---|---|
| AC-GO-01 工程基线 | GO-1.1～GO-1.6、GO-6.1 | test/vet/race 输出 |
| AC-GO-02 空库迁移 | GO-2.1～GO-2.5 | Testcontainers migration 记录 |
| AC-GO-03 API 兼容 | GO-0.2、GO-0.3、GO-6.2 | OpenAPI 与 fixture 回归 |
| AC-GO-04 分类统一管理 | GO-3.3、GO-3.6 | React + 小程序目录 E2E |
| AC-GO-05 SKU 发布隔离 | GO-3.4、GO-3.5 | 工作副本/发布版本集成测试 |
| AC-GO-06 媒体安全 | GO-3.1、GO-3.2、GO-6.4 | 跨客户和流式上传测试 |
| AC-GO-07 幂等订单 | GO-5.1、GO-5.2、GO-6.3 | 并发 MySQL 测试 |
| AC-GO-08 调价保护 | GO-4.2、GO-5.2 | `CART_SKU_CHANGED` E2E |
| AC-GO-09 Java 退出 | GO-7.1～GO-7.4 | 最终构建、Compose 和依赖检查 |

## 8. SPEC-001 追踪矩阵

| SPEC-001 验收 | Go 任务 |
|---|---|
| AC-01 草稿不可见 | GO-3.4～GO-3.6 |
| AC-02 发布后可见 | GO-3.2、GO-3.5、GO-3.6 |
| AC-03 正常加购 | GO-4.1、GO-4.2 |
| AC-04 正常下单 | GO-5.1～GO-5.3 |
| AC-05 后台可见 | GO-5.4 |
| AC-06 防重复订单 | GO-5.1、GO-6.3 |
| AC-07 调价保护 | GO-3.5、GO-5.2 |
| AC-08 故障资料必填 | GO-4.3、GO-5.2 |
| AC-09 私有媒体 | GO-3.2、GO-6.4 |
| AC-10 分类统一管理 | GO-3.3、GO-3.6 |
| AC-11 分类停用保护 | GO-3.3 |
| AC-12 三项导航 | Go 公共目录契约 + 现有小程序回归 |

## 9. 风险与控制

| 风险 | 控制措施 |
|---|---|
| 边迁移边改 API | G0 冻结 OpenAPI 与 fixture；三端适配层不为 Go 特判 |
| Java/Go 双写 | 独立数据库、独立端口，切流维护窗口只保留一个写者 |
| Flyway/migrate 冲突 | Go 数据库只运行 `schema_migrations`；不复制 Flyway 历史 |
| 直接 ORM 自动建表 | 禁止 GORM AutoMigrate；Schema 只来自 SQL migration |
| sqlc 类型映射错误 | 显式 override，覆盖 BIGINT、JSON、DATETIME 和 nullable 字段测试 |
| 进程锁伪幂等 | 数据库唯一约束、行锁和事务；多实例并发测试 |
| 视频导致 OOM | io.Reader 流式复制、请求上限和内存基准测试 |
| Go 错误信息泄露 SQL | 统一错误映射，未知错误只记录内部日志 |
| local Token 进入生产 | production 启动阻断与 CI 配置测试 |
| 删除 Java 后难回查 | 删除前打迁移完成 Tag/提交，保留契约和验收报告 |
| Go 写入后直接回 Java | 明确禁止；使用前向修复或维护窗口数据回灌 |

## 10. 执行命令基线

Go 工程：

```powershell
cd apps/server-go
go version
go mod download
go generate ./...
go fmt ./...
go vet ./...
go test ./...
go test -race ./...
```

本地基础设施：

```powershell
docker compose -f deploy/compose.yaml up -d mysql redis
```

Go migration：

```powershell
go run ./cmd/migrate up
go run ./cmd/migrate version
```

启动候选服务：

```powershell
$env:APP_ENV='local'
$env:HTTP_ADDR=':8081'
$env:DB_DSN='fixpro:fixpro-local@tcp(localhost:3306)/fix_pro_go?parseTime=true&charset=utf8mb4&loc=UTC'
go run ./cmd/api
```

三端检查：

```powershell
npm run check
```

最终切流前：

```powershell
docker build -t fixpro-server-go:local apps/server-go
```

## 11. 完成判定

只有同时满足以下条件，PLAN-002 才算完成：

- SPEC-002 AC-GO-01～AC-GO-09 全部通过。
- SPEC-001 AC-01～AC-12 全部在 Go 服务上通过。
- React 和小程序 API 适配层没有 Go 专属分支。
- 空库 migration、sqlc 检查、真实 MySQL 集成测试通过。
- `go test、go test -race、go vet` 和 Docker build 通过。
- 分类、SKU 发布、购物车和订单并发语义由数据库保证。
- 故障媒体跨客户访问被拒绝，视频流式处理验证通过。
- Go production 配置拒绝默认密码、本地 Token 和 local media。
- 切流前数据库有备份，数据接管方式有记录。
- Compose、CI、README 和运行手册只指向 Go 后端。
- 最终 `apps/server` 不再包含 Java、Maven、Spring Boot 和 Flyway 运行依赖。

## 12. 明确不在本计划内

- 支付、退款、优惠券和发票；
- 预约、派单、师傅完整履约；
- ToB 合同、项目、额度、SLA 和批量工单；
- 微服务、消息队列和分布式事务；
- 正式微信 OAuth 和生产员工 IAM；
- Elasticsearch 或独立搜索服务；
- 为迁移而重做 React 或小程序视觉；
- 在 Go 验收前删除 Java 或现有数据库。

新增业务需求应先更新产品方案和 Spec，不得混入语言迁移提交。
