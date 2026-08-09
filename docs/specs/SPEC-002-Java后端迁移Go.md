# SPEC-002｜FixPro 后端从 Java 迁移到 Go

**状态：** Draft for review  
**版本：** V1.0  
**日期：** 2026-08-04  
**上游依据：** 产品方案 V1.3、技术方案 V1.3、设计方案 V1.3、SPEC-001 V1.3  
**迁移范围：** `apps/server` Java Spring Boot 后端 → Go 模块化单体  
**不变范围：** `apps/admin-web` React、`apps/wechat-mini` 微信原生小程序、MySQL 业务模型与 HTTP API 语义

---

## 1. 迁移结论

FixPro 后端改为 Go，但本次迁移是技术实现替换，不是产品重做。

必须遵守以下原则：

1. React 管理后台和微信小程序继续使用当前 API 路径、请求字段、响应结构、错误码和鉴权语义。
2. 继续采用模块化单体，不因为改用 Go 提前拆微服务。
3. 保留 MySQL、Redis 和对象存储边界；第一期正向链路继续优先保证强事务一致性。
4. Go 服务达到 SPEC-001 AC-01～AC-12 等价能力后才能切流。
5. 迁移期间 Java 与 Go 不得同时写入同一个业务数据库，也不得同时管理同一套数据库迁移版本。
6. Java 实现只作为接口行为参考；Go 验收通过后从主工程删除，历史通过 Git 保留，不在仓库长期维护双栈。

## 2. 背景与当前基线

当前工程已经具备：

- Java 21 + Spring Boot 3.5 的初始化后端；
- MySQL V1/V2 SQL，包括组织、客户、分类、SKU、SKU 版本、媒体、购物车、订单和幂等记录；
- React SKU/分类/订单管理页面；
- 小程序“首页、全部服务、我的”、搜索、详情、购物车、故障资料和下单页面；
- 前端 API 统一响应：`code、message、data、requestId`；
- 管理后台本地 Basic Auth 与小程序 local Bearer Token。

当前 Java 后端尚未进入生产，也没有需要无损保留的生产数据。因此本 Spec 默认采用“本地/测试数据库重建后由 Go migration 接管”的方式。如果执行迁移前发现已经存在必须保留的共享或生产数据，必须暂停重建，另行制定数据接管方案。

## 3. 目标与非目标

### 3.1 目标

- 使用 Go 实现当前全部后端接口，并通过前端不改契约的端到端回归。
- 建立适合小团队长期维护的 Go 工程结构、数据库访问、迁移、日志、测试和 CI 基线。
- 保证 SKU 发布、购物车、订单、媒体和幂等事务语义不下降。
- 保证分类由后台统一管理，SKU 与小程序目录读取同一事实来源。
- 保证故障图片/视频仍是私有媒体，不能因迁移出现永久公共地址。
- 迁移完成后只保留一个生产后端和一个数据库迁移工具。

### 3.2 非目标

- 不增加支付、退款、预约、派单、企业合同、SLA 或完整订单履约。
- 不重写 React 或微信小程序页面。
- 不改变 API 版本号 `/api/v1`。
- 不把模块化单体拆成微服务。
- 不在本期引入 Kafka、分布式事务、服务网格或 Kubernetes。
- 不借迁移机会修改 SKU 定价、业务状态或首批服务范围。

## 4. 技术基线

### 4.1 语言与运行时

- Go `1.26.x`，CI 与 Docker 镜像固定具体补丁版本。
- Linux amd64 为生产默认目标，开发环境支持 Windows amd64。
- 使用 Go Modules；必须提交 `go.mod` 和 `go.sum`。
- 构建产物为单一静态优先的 API 二进制，不在生产镜像中携带编译工具链。

### 4.2 HTTP

- 使用标准库 `net/http` 和 Go 1.26 `http.ServeMux` 路由能力。
- 不使用重量级 Web 框架；中间件以标准 `http.Handler` 组合。
- JSON 使用 `encoding/json`。
- API 契约维护在 `api/openapi.yaml`；是否使用生成器只生成 DTO/接口，不允许生成器承载业务逻辑。
- 服务端统一配置请求头、读取、写入和空闲超时，并支持优雅停机。

### 4.3 数据库访问

- MySQL 8.4。
- 使用标准库 `database/sql`。
- MySQL Driver 使用 `github.com/go-sql-driver/mysql`，DSN 必须启用 `parseTime=true`、UTF-8 和明确时区。
- SQL 查询使用 `sqlc` 生成类型安全 Go 代码；复杂动态查询允许在 Repository 中显式编写，但必须参数化，禁止字符串拼接用户输入。
- 不使用 GORM 自动建表，不允许应用启动时根据 struct 自动修改 Schema。
- 金额统一使用 `int64` 分；数据库 ID 使用 `uint64/int64`，JSON 对外序列化为字符串。

### 4.4 数据库迁移

- 使用 `golang-migrate/migrate/v4`。
- 迁移文件放在 `db/migrations`，命名：

```text
000001_baseline.up.sql
000001_baseline.down.sql
000002_sku_cart_order_slice.up.sql
000002_sku_cart_order_slice.down.sql
```

- 生产部署默认只执行 `up`；`down` 只用于本地开发，且不得假设生产数据可以无损回滚。
- Go 服务使用 `schema_migrations`；不继续使用 `flyway_schema_history`。
- Flyway 和 golang-migrate 不能同时对同一个数据库运行。

### 4.5 日志、指标与追踪

- 使用标准库 `log/slog` 输出 JSON 结构化日志。
- 每个请求生成或透传 `X-Request-Id`。
- 日志字段至少包含 `requestId、method、path、status、durationMs、principalType、principalId`。
- 禁止记录完整手机号、Authorization、密码、故障媒体地址和上传文件内容。
- 健康接口：`GET /actuator/health` 在迁移期保持兼容；内部实现不保留 Spring 语义。
- 指标接口可使用 `/metrics`，但不作为本次切流阻塞项；健康检查是阻塞项。

## 5. 目标工程结构

迁移开发阶段新建 `apps/server-go`，避免未完成实现破坏当前 Java 参考服务。切流完成后将其作为 `apps/server`，删除 Java 工程文件。

```text
apps/server-go/
├─ cmd/
│  ├─ api/main.go
│  └─ migrate/main.go
├─ api/
│  └─ openapi.yaml
├─ db/
│  ├─ migrations/
│  ├─ queries/
│  └─ sqlc.yaml
├─ internal/
│  ├─ app/
│  │  ├─ bootstrap.go
│  │  └─ routes.go
│  ├─ platform/
│  │  ├─ config/
│  │  ├─ database/
│  │  ├─ httpx/
│  │  ├─ auth/
│  │  ├─ idempotency/
│  │  ├─ media/
│  │  └─ observability/
│  ├─ catalog/
│  │  ├─ handler.go
│  │  ├─ service.go
│  │  ├─ repository.go
│  │  └─ model.go
│  ├─ cart/
│  ├─ order/
│  └─ media/
├─ internal/dbgen/          # sqlc 生成代码，不手工修改
├─ test/
│  ├─ integration/
│  └─ contract/
├─ Dockerfile
├─ go.mod
├─ go.sum
├─ Makefile
└─ README.md
```

模块规则：

- Handler 只处理 HTTP、认证主体、DTO 映射和错误输出。
- Service 承载用例、事务和业务校验。
- Repository 封装 SQL，不向 Handler 暴露数据库模型。
- Catalog、Cart、Order 和 Media 不互相直接访问对方表；跨模块读取通过窄接口。
- `internal/dbgen` 只包含生成代码，业务代码不能手改生成文件。

## 6. 配置规范

全部配置通过环境变量注入，不在代码中保存密钥：

| 环境变量 | 说明 |
|---|---|
| `APP_ENV` | `local/test/staging/production` |
| `HTTP_ADDR` | 默认 `:8080` |
| `DB_DSN` | MySQL DSN |
| `DB_MAX_OPEN_CONNS` | 最大连接数 |
| `DB_MAX_IDLE_CONNS` | 最大空闲连接数 |
| `REDIS_ADDR` | Redis 地址 |
| `MEDIA_DRIVER` | `local/s3` |
| `MEDIA_LOCAL_ROOT` | 本地媒体根目录 |
| `APP_ADMIN_USERNAME` | 本地 Bootstrap 管理员 |
| `APP_ADMIN_PASSWORD` | 本地 Bootstrap 密码 |
| `CORS_ALLOWED_ORIGINS` | React 后台来源白名单 |

启动时必须校验必要配置；production 中出现默认密码、本地客户 Token 或 `MEDIA_DRIVER=local` 时启动失败。

## 7. API 兼容契约

### 7.1 统一响应

成功：

```json
{
  "code": "OK",
  "message": "success",
  "data": {},
  "requestId": "..."
}
```

失败：

```json
{
  "code": "CATEGORY_IN_USE",
  "message": "分类下存在已发布 SKU，请先下架或移动服务",
  "data": null,
  "requestId": "..."
}
```

Go 实现必须保持当前错误码、HTTP 状态、字段大小写、空值语义和金额单位。

### 7.2 必须等价实现的接口

系统与媒体：

- `GET /actuator/health`
- `GET /api/v1/public/ping`
- `POST /api/v1/admin/media/images`
- `POST /api/v1/mini/media/fault`
- `GET /api/v1/public/media/{id}`
- `GET /api/v1/admin/media/{id}/content`
- `GET /api/v1/mini/media/{id}/content`
- `DELETE /api/v1/admin/media/{id}`
- `DELETE /api/v1/mini/media/{id}`

分类与 SKU：

- `GET /api/v1/admin/catalog/categories`
- `POST /api/v1/admin/catalog/categories`
- `PUT /api/v1/admin/catalog/categories/{id}`
- `POST /api/v1/admin/catalog/categories/{id}/status`
- `GET /api/v1/admin/catalog/skus`
- `GET /api/v1/admin/catalog/skus/{id}`
- `POST /api/v1/admin/catalog/skus`
- `PUT /api/v1/admin/catalog/skus/{id}`
- `POST /api/v1/admin/catalog/skus/{id}/publish`
- `POST /api/v1/admin/catalog/skus/{id}/off-shelf`
- `GET /api/v1/catalog/categories`
- `GET /api/v1/catalog/services?keyword=`
- `GET /api/v1/catalog/services/{id}`

购物车与订单：

- `GET /api/v1/mini/cart`
- `POST /api/v1/mini/cart/items`
- `PATCH /api/v1/mini/cart/items/{itemId}`
- `PUT /api/v1/mini/cart/items/{itemId}/fault`
- `DELETE /api/v1/mini/cart/items/{itemId}`
- `POST /api/v1/mini/orders`
- `GET /api/v1/admin/orders`
- `GET /api/v1/admin/orders/{id}`

### 7.3 鉴权兼容

- `/api/v1/catalog/**` 和已发布 SKU 公共图片允许匿名 GET。
- `/api/v1/admin/**` 本地阶段继续支持 Basic Auth 并要求管理员角色。
- `/api/v1/mini/**` 需要客户身份。
- `APP_ENV=local` 时支持 `Bearer local-customer-1` → `orgId=1/customerId=1`。
- 非 local 环境不得注册或接受本地 Token。
- 客户 ID、组织 ID 不从请求体或自定义 Header 直接信任。

## 8. 领域行为要求

### 8.1 Catalog

- 分类支持新增、编辑、排序、启停和 SKU 数量统计。
- 存在已发布 SKU 的分类不能停用，返回 `CATEGORY_IN_USE`。
- SKU 工作副本与发布快照分离。
- 公共目录只读取当前发布版本，不读取未发布修改。
- SKU 发布在同一事务生成不可变版本、切换当前版本并写审计/Outbox。
- 服务范围、除外项、售后/质保、图片顺序必须进入发布快照。

### 8.2 Cart

- 购物车按 `(org_id, customer_id)` 唯一。
- 重复加购同一 SKU 版本累计数量，范围 1～99。
- 金额由服务端按整数分计算。
- 每个购物车项独立保存故障描述和媒体。
- 跨客户读取、修改或关联购物车/媒体返回 403 或业务错误。

### 8.3 Order

- 创建订单必须使用 `Idempotency-Key`。
- 订单、订单项、媒体关联、幂等结果和购物车清理必须在一个 MySQL 事务中。
- 下单前锁定购物车，重新校验 SKU 状态、版本、价格和故障资料。
- 调价或重新发布后返回 `CART_SKU_CHANGED`，不静默成交、不清空资料。
- 订单保存名称、编码、价格、服务承诺、SKU 图片、故障描述和媒体快照。
- 相同幂等键与相同请求返回首次结果；相同键不同请求返回 `ORDER_SUBMIT_DUPLICATED`。

### 8.4 Media

- 定义 `ObjectStorage` 接口；local 使用文件系统，production 使用 S3/COS/OSS 兼容实现。
- 上传和下载流式处理，禁止将 50 MB 视频整体读入内存。
- 图片允许 JPEG/PNG/WebP，视频允许 MP4/MOV；同时校验 MIME、文件签名、大小和数量。
- SKU 发布图片可以公开；故障媒体只能由所属客户和有权限管理员读取。
- 随机生成对象 Key，不能拼接原始文件名或客户端路径。

## 9. 事务与并发

- Service 通过 `database/sql` 的 `BeginTx` 显式控制事务。
- 所有事务函数第一参数为 `context.Context`，并在超时/取消时回滚。
- 事务内 Repository 使用同一个 `*sql.Tx` 或 sqlc `WithTx`。
- SKU 编辑使用 `version` 乐观锁。
- 下单使用购物车行锁与幂等唯一约束，不能只依赖进程内 `sync.Mutex`。
- 单实例和多实例部署必须产生相同幂等结果。
- 订单号和唯一约束由数据库最终防重。

## 10. 数据库接管方案

### 10.1 默认方案：无生产数据

适用于当前本地/测试工程：

1. 停止 Java 和 Go 服务。
2. 备份当前数据库，仅用于问题追查。
3. 删除本地/测试 `fix_pro` 数据库或 Compose MySQL 卷。
4. 使用 Go migration 从 `000001` 重建。
5. 检查 `schema_migrations` 版本为最新。
6. 启动 Go 服务，执行种子与正向链路验收。

### 10.2 已存在需保留数据时

如果发现共享、演示或生产数据必须保留：

- 禁止删除数据库或 Flyway 历史表。
- 对 V1/V2 Schema 做逐表校验和数据备份。
- 建立一次性 baseline：确认现有 Schema 与 Go `000002` 完全一致后，将 golang-migrate 标记到对应版本。
- baseline 工具必须输出检查报告，任何列、索引、字符集或约束不一致时失败，不得强行标记版本。
- Flyway 历史表只归档，不由 Go 运行时继续写入。

## 11. 错误处理

- 领域错误使用可比较的错误码，不用字符串解析判断类型。
- Handler 统一映射 HTTP 状态和 `ApiResponse`。
- 未知错误返回 `INTERNAL_ERROR`，对客户端隐藏堆栈和数据库信息。
- 日志保留包裹后的根因和 requestId。
- `context.Canceled` 与客户端断开不记录为服务端 500。
- MySQL duplicate key、deadlock、lock timeout 必须分别映射或有限重试；订单提交禁止无限重试。

## 12. 安全要求

- HTTP Server 必须设置 `ReadHeaderTimeout、ReadTimeout、WriteTimeout、IdleTimeout` 和最大 Header 大小。
- JSON 请求体设置大小上限，并拒绝未知关键字段或在契约测试中固定兼容策略。
- SQL 全部参数化。
- CORS 只允许配置白名单。
- 管理员密码使用常量时间校验；生产阶段切换正式员工认证前，Basic Auth 不得暴露公网。
- 私有媒体响应使用 `Cache-Control: no-store` 和 `X-Content-Type-Options: nosniff`。
- 公共图片只有被当前已发布 SKU 引用时才可读取。
- production 启动时检查默认密码、本地 Token 和本地媒体驱动。

## 13. 测试策略

### 13.1 单元测试

- Catalog 发布校验与快照。
- 分类停用保护。
- Cart 数量、合计和资料完整性。
- Order 金额、幂等请求哈希和状态冲突。
- 媒体类型、签名、大小和对象 Key。

### 13.2 Repository 与事务集成测试

- 使用 Testcontainers for Go 启动真实 MySQL 8.4。
- 每个测试套件从 golang-migrate 空库迁移。
- 覆盖唯一约束、JSON、行锁、乐观锁、事务回滚、死锁/并发提交和幂等。
- 不使用 SQLite 替代最终 MySQL 语义。

### 13.3 HTTP 契约测试

- 以 `api/openapi.yaml` 和固定 JSON fixture 校验字段、类型、状态码和错误码。
- 对 Java 参考实现和 Go 实现执行同一套只读/可重建数据契约测试。
- React 和小程序不得通过“适配 Go 特殊返回”来绕过不兼容。

### 13.4 端到端测试

完整执行 SPEC-001 AC-01～AC-12：

```text
后台维护分类
→ 上传图片并发布“家庭基础漏水检测”
→ 小程序首页/全部服务展示
→ 加入购物车并上传故障资料
→ 提交 99 元测试订单
→ 后台查看订单和不可变快照
```

## 14. 性能与可靠性基线

- 无上传的普通 API 在测试环境 p95 小于 300 ms。
- 目录列表 100 个 SKU 时 p95 小于 200 ms。
- 50 MB 视频上传不导致进程内存随文件大小等比例增长。
- 服务收到终止信号后停止接收新请求，并在最多 15 秒内完成或取消在途请求。
- MySQL 连接池必须设置最大连接、空闲连接、连接最大生命周期和健康检查。
- 所有外部调用必须携带 context timeout。
- `go test -race ./...` 必须通过。

## 15. CI 与构建

根工程新增：

```text
npm run check
go fmt check
go vet ./...
go test ./...
go test -race ./...
sqlc diff/check
golang-migrate empty database up
docker build apps/server-go
```

Docker 使用多阶段构建：

- build 阶段固定 Go 1.26.x；
- runtime 使用无 Shell 或最小化非 root 镜像；
- 二进制以非 root 用户运行；
- 镜像内不包含源码、Go 缓存和测试密钥。

## 16. 迁移实施顺序

### Phase G0｜冻结契约

- 从当前 React、小程序和 SPEC-001 固化 OpenAPI 与 JSON fixture。
- 记录全部错误码、鉴权规则、金额和 ID 表示。
- Java 进入功能冻结，只接受迁移阻塞修复。

### Phase G1｜Go 工程与平台底座

- 初始化 `apps/server-go`、配置、HTTP Server、requestId、日志、健康检查和优雅停机。
- 接入 MySQL、sqlc、golang-migrate。
- 建立管理员 Basic Auth 和 local 客户认证。

### Phase G2｜数据库迁移接管

- 将 V1/V2 SQL 转换为 golang-migrate up/down 文件。
- 建立空库迁移与 Schema 校验测试。
- 确认默认重建方案或触发保留数据方案。

### Phase G3｜Catalog 与 Media

- 分类 CRUD/启停。
- SKU 草稿、版本发布、下架和公共目录。
- SKU 图片与私有故障媒体。
- React SKU/分类和小程序首页/全部服务契约回归。

### Phase G4｜Cart 与 Order

- 服务端购物车、故障资料、价格版本校验。
- 幂等订单事务和后台订单查询。
- 完成正向链路。

### Phase G5｜对比与切流

- 执行契约、集成、安全和 SPEC-001 全量验收。
- Compose 和本地代理切到 Go 服务。
- 观察 Go 日志、错误率和数据库连接。
- 验收通过后删除 Java 构建、Flyway 配置和 Java CI。

### Phase G6｜收尾

- `apps/server-go` 归位为 `apps/server`。
- 更新 README、运行手册、技术方案和 ADR。
- 删除便携 JDK、Maven 缓存和 Java 专用临时文件。
- 保留 API 契约 fixture 作为后续回归资产。

## 17. 验收场景

### AC-GO-01 工程基线

```gherkin
Given 开发机安装 Go 1.26.x
When 执行 go test ./... 和 go vet ./...
Then 全部通过
And 不需要 JDK 或 Maven
```

### AC-GO-02 空库迁移

```gherkin
Given 一个空 MySQL 8.4 数据库
When 执行 Go migration up
Then 全部表、索引和种子数据创建成功
And schema_migrations 记录最新版本
And 不创建 flyway_schema_history
```

### AC-GO-03 API 兼容

```gherkin
Given 当前 React 和微信小程序代码不修改 API 适配层
When API Base URL 切换到 Go 服务
Then 登录、分类、SKU、媒体、购物车和订单请求正常
And 响应字段、错误码和金额单位不变
```

### AC-GO-04 分类统一管理

```gherkin
Given 管理员通过 Go API 新增并启用分类
When 打开 SKU 表单和小程序全部服务
Then 两端都读取到该分类
And 有已发布 SKU 时停用返回 CATEGORY_IN_USE
```

### AC-GO-05 SKU 发布隔离

```gherkin
Given 一个已发布 SKU
When 管理员只修改工作副本但未再次发布
Then 小程序仍看到原发布版本
When 再次发布
Then 小程序看到新版本
And 历史订单不变化
```

### AC-GO-06 媒体安全

```gherkin
Given 客户 A 上传故障视频
When 客户 B 请求该视频
Then 返回 403 和 MEDIA_ACCESS_DENIED
And 视频上传过程不整体载入内存
```

### AC-GO-07 幂等订单

```gherkin
Given 同一客户购物车资料完整
When 两个并发请求使用相同 Idempotency-Key 提交订单
Then 数据库只存在一张订单
And 两次得到同一业务结果或明确的处理中结果
```

### AC-GO-08 调价保护

```gherkin
Given 客户加入 99 元版本
And 管理员发布 129 元新版本
When 客户提交订单
Then 返回 CART_SKU_CHANGED
And 不创建订单
And 购物车故障资料保留
```

### AC-GO-09 Java 退出

```gherkin
Given Go 服务已通过全部验收并切流
When 检查生产构建与部署
Then 不再需要 JDK、Maven、Spring Boot 或 Flyway
And 只有 Go 服务拥有业务数据库写权限
```

## 18. Definition of Done

- AC-GO-01～AC-GO-09 全部通过。
- SPEC-001 AC-01～AC-12 在 Go 服务上全部通过。
- React 与微信小程序 API 适配层无需为 Go 修改字段或错误码。
- 空库 migration、真实 MySQL 集成测试、race test、vet 和 Docker build 通过。
- Go 实现不存在进程内锁替代数据库幂等的情况。
- 故障媒体无永久公开地址，上传采用流式处理。
- production 配置拒绝默认密码、本地 Token 和本地媒体目录。
- Compose、CI、README 和运行手册全部切换到 Go。
- Java、Maven、Spring Boot 和 Flyway 从最终生产构建移除。
- 数据库接管方式及备份结果有记录，可明确回答是否保留过旧环境数据。

## 19. 风险与控制

| 风险 | 影响 | 控制措施 |
|---|---|---|
| 改语言时顺便改 API | 三端同时返工 | 先冻结 OpenAPI/fixture，Go 做等价实现 |
| Flyway 与 migrate 同时运行 | Schema 版本损坏 | 同库只能启用一个迁移工具，切换前停服务 |
| 用进程锁实现幂等 | 多实例重复订单 | 数据库唯一约束、事务和行锁作为最终保证 |
| sqlc 生成模型泄露到 API | 数据库变更扩散 | DTO、领域模型和 dbgen 分离 |
| 视频读入内存 | OOM | multipart 限制、流式 copy、内存测试 |
| local Token 进入生产 | 客户越权 | APP_ENV 条件注册与生产启动检查 |
| 双栈长期共存 | 修复重复、行为漂移 | 明确切流门槛，完成后删除 Java 主线 |
| 现有数据被误删 | 不可恢复 | 默认重建仅限无生产数据；执行前备份与确认 |

## 20. 参考基线

- Go 1.26 是当前稳定主版本，Go 官方按“最近两个主版本”提供支持。
- sqlc 为 MySQL 生成类型安全 Go 数据访问代码。
- golang-migrate v4 支持 MySQL、文件迁移源及 Go/CLI 两种运行方式。
- Testcontainers for Go 用于真实 MySQL 集成测试。
