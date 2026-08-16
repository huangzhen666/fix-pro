# PLAN-010｜师傅账号登录、首次改密与作业权限体系实施计划

**状态：** Executed（A0—A8 已完成；生产环境发布与微信开发者工具人工验收待部署窗口执行）  
**版本：** V1.0  
**日期：** 2026-08-16  
**对应 Spec：** `docs/specs/SPEC-010-师傅账号登录与作业权限体系.md` V1.0  
**适用工程：** `apps/server-go`、`apps/admin-web`、`apps/wechat-worker-mini`、`apps/server-go/db/migrations`  
**执行顺序：** 契约冻结 → 数据库与存量账号 → Worker 认证 → Worker 接口收口 → 后台重置密码 → 师傅小程序登录 → 联调与安全验收

---

## 1. 交付目标

完成师傅账号从创建、登录、首次改密、作业授权到后台重置密码的完整链路：

```text
后台新增/启用师傅
→ 系统生成 w+手机号 初始密码
→ 师傅使用手机号登录作业小程序
→ 首次登录强制修改密码
→ 只查看和操作本人被派单的工单
→ 后台可二次确认重置密码
→ 重置后旧会话失效并再次强制改密
```

完成时必须满足：

- 师傅账号登录名统一为手机号，师傅编号只作为业务编号；
- 初始密码和后台重置密码均为 `w+当前手机号`，数据库只保存 Argon2id 哈希；
- 首次登录、密码重置和手机号变更后必须改密；
- Worker 使用独立会话，不复用管理员或客户登录态；
- 所有 Worker API 按 `org_id + assignee_id` 在服务端隔离数据；
- 后台重置密码有专用权限、二次确认、一次性密码展示和审计；
- 生产环境不再接受 `local-worker-*` 演示 Token；
- SPEC-010 的验收用例全部通过。

---

## 2. 当前基线与实施边界

### 2.1 当前基线

- PostgreSQL migration 已存在 `employee_account` 和师傅管理相关字段；
- 当前 Worker middleware 仍解析 `local-worker-{id}`；
- 师傅工单接口已经存在，但需要统一切换到真实会话 Principal；
- `employee_account.password_hash` 可能仍是不可登录占位值；
- 管理后台已接入 `admin_user`、Casbin 和 Argon2id；
- 师傅小程序已存在工作台、工单、详情和我的页面，但没有登录页和改密页；
- 历史 migration 不允许修改，本计划新增 `000013_worker_auth`。

### 2.2 实施边界

本计划包含：

- Worker 账号字段、session 表和存量账号迁移；
- Worker 登录、当前身份、登出、修改密码和会话中间件；
- Worker 所有工单和媒体接口的身份隔离；
- 后台师傅重置密码、创建/编辑手机号时的密码状态处理；
- Casbin `worker.reset_password` 权限和审计；
- 师傅小程序登录和首次改密页面；
- 后端、小程序和后台的联调、安全与 E2E 验收。

明确不做：

- 师傅自主注册、短信验证码、微信手机号授权登录；
- 师傅自行找回密码；
- 客户小程序和师傅小程序共享 Token；
- 师傅在小程序修改手机号、姓名、工种、技能和证书；
- 本计划之外的订单、履约状态机重构。

---

## 3. 实施原则

1. 不修改已执行 migration，只新增 `000013_worker_auth.up.sql` 和对应 down migration。
2. 先建立数据结构和认证核心，再切换业务接口，最后改造前端入口。
3. Worker 身份只来源于服务端会话，不信任 URL、Query、Body 或自定义 Header 中的师傅 ID。
4. 所有业务查询同时校验 `org_id` 和 `assignee_id`，前端隐藏不作为安全措施。
5. 密码和 Token 不写日志；重置密码的明文只在本次后台响应中返回一次。
6. 认证失败、状态不可用和资源越权使用统一且可读的中文错误提示。
7. 每个里程碑完成真实 PostgreSQL 和自动化验证后再进入下一阶段。
8. 只改动本功能直接涉及的代码，不顺带重构现有订单和履约模块。

---

## 4. 里程碑与关键路径

| 里程碑 | 交付结果 | 主要验证 | 依赖 |
| --- | --- | --- | --- |
| A0 | 账号、接口、错误码和权限契约冻结 | OpenAPI、路由和测试矩阵完整 | SPEC-010 |
| A1 | PostgreSQL 账号字段、session 表和存量数据可用 | 空库迁移、升级迁移、唯一约束和回滚检查 | A0 |
| A2 | Worker 登录、改密、登出和会话中间件可用 | 登录、Argon2id、过期、撤销、423 首次改密 | A1 |
| A3 | 所有 Worker 业务接口完成真实身份和越权收口 | 师傅隔离、跨组织和媒体权限测试 | A2 |
| A4 | 后台师傅重置密码闭环可用 | Casbin、二次确认、一次性密码和审计 | A1、A2 |
| A5 | 师傅小程序登录和首次改密可用 | 401/423 跳转、中文提示、Token 生命周期 | A2、A3 |
| A6 | 三端联调和存量账号迁移完成 | 新增、重置、登录、改密、工单正向链路 | A3、A4、A5 |
| A7 | 安全、并发、性能和回归验收完成 | SPEC-010 全部验收用例通过 | A6 |
| A8 | 文档、配置和生产收口完成 | 关闭本地 Token，启动和回滚说明完整 | A7 |

关键路径：

```text
A0 → A1 → A2 → A3 → A5 → A6 → A7 → A8
           └──────→ A4 ────────┘
```

---

## 5. A0｜契约、权限和测试骨架

### A0-1｜盘点现有实现

检查并记录：

- `apps/server-go/internal/platform/auth/auth.go` 的 Worker middleware；
- `apps/server-go/internal/app/app.go` 的全部 `/api/v1/worker/**` 路由；
- `apps/server-go/internal/workforce` 和 `internal/fulfillment` 中的师傅创建、编辑、状态操作；
- `apps/wechat-worker-mini/miniprogram/app.ts`、请求层和所有页面；
- `apps/admin-web` 师傅管理页面、权限编码和 API 客户端；
- `employee_account` 当前状态、用户名、手机号和密码占位数据。

验证产物：一份 Worker 路由清单，标明认证要求、资源归属条件、允许的错误码和测试用例。

### A0-2｜冻结接口契约

在 Go Handler、前端 API 服务和 `apps/server-go/api/openapi.yaml` 中统一定义：

- `POST /api/v1/worker/auth/login`；
- `GET /api/v1/worker/auth/me`；
- `POST /api/v1/worker/auth/logout`；
- `POST /api/v1/worker/auth/password`；
- `POST /api/v1/admin/workers/{id}/reset-password`；
- `worker.reset_password` 权限编码；
- `WORKER_LOGIN_FAILED`、`WORKER_SESSION_INVALID`、`WORKER_PASSWORD_CHANGE_REQUIRED` 等错误结构。

验证：接口请求/响应字段、HTTP 状态码、中文提示和前端类型定义一致，不在页面代码中自行拼装错误码。

### A0-3｜建立测试夹具

准备：

- 两个组织；
- 两名 ACTIVE 师傅，手机号不同；
- DRAFT、DISABLED 和无手机号师傅；
- 两名师傅分别拥有自己的工单；
- 一个具有 `worker.reset_password` 的管理员和一个没有该权限的管理员；
- 可重复清理的 Worker session 和审计数据。

退出条件：后续测试可以不依赖手工插入明文密码或本地 Token。

---

## 6. A1｜PostgreSQL 模型与存量账号迁移

### A1-1｜新增 migration

新增：

```text
apps/server-go/db/migrations/000013_worker_auth.up.sql
apps/server-go/db/migrations/000013_worker_auth.down.sql
```

Up migration：

- 为 `employee_account` 增加 `must_change_password`、`last_login_at`、`last_password_changed_at`、`password_version`；
- 确认同组织手机号唯一约束；
- 新建 `worker_session`，包含 `org_id`、`worker_id`、Token 哈希、密码版本、过期和撤销字段；
- 增加 Token 哈希唯一约束及 `(org_id, worker_id, revoked_at, expires_at)` 索引；
- 使用 CHECK、外键和 NOT NULL 约束保证状态和组织关系正确。

down migration 只回滚本次新增结构，不恢复明文密码。

### A1-2｜迁移存量 Worker

编写一次性安全迁移或可审计脚本：

- 对 `role=WORKER` 且手机号有效的账号生成 Argon2id(`w+手机号`)；
- 设置 `username=手机号`、`must_change_password=true`、`password_version=1`；
- 无手机号或手机号非法的账号保持不可登录，并在后台提示先完善手机号；
- 不在 SQL、脚本输出和日志中打印明文密码；
- 重复执行必须幂等，不能覆盖已经完成正式改密的师傅密码。

验证：新库、存量升级、重复执行和失败回滚均在真实 PostgreSQL 完成。

### A1-3｜数据库并发约束

- 新增和修改手机号使用唯一索引兜底，并将冲突映射为 `409 WORKER_MOBILE_EXISTS`；
- 重置密码和修改密码在事务中更新密码版本并撤销 session；
- 同一师傅并发重置只能产生一致的当前手机号默认密码；
- session 清理使用索引，避免每次登录扫描全部会话。

退出条件：数据库迁移成功，约束、索引、存量账号状态和数据快照均可验证。

---

## 7. A2｜Go Worker 认证、会话和密码服务

### A2-1｜实现密码服务

在 `apps/server-go/internal/platform/auth` 或独立认证包中复用官方 Argon2id 实现：

- `HashPassword`；
- `ComparePassword`；
- 初始密码 `w+手机号` 生成；
- 新密码强度和禁止使用初始密码校验；
- 统一错误映射。

测试：密码哈希不可逆、相同密码每次盐不同、错误密码不能通过、历史占位哈希不能登录。

### A2-2｜实现 Worker session repository/service

实现：

- 随机 Token 生成和 SHA-256 哈希保存；
- 创建、查询、撤销单个和全部 session；
- 过期判断、密码版本判断和账号状态判断；
- 登录时间与最近使用时间更新；
- 密码变更、重置、禁用时的会话撤销。

建议文件范围：

```text
apps/server-go/internal/platform/auth/worker_session.go
apps/server-go/internal/platform/auth/worker_password.go
apps/server-go/internal/platform/auth/worker_repository.go
```

不得把原始 Token 写入数据库或日志。

### A2-3｜实现认证接口

在 `internal/workforce/handler.go`、`service.go` 或专用 `worker_auth` 模块实现：

- `POST /api/v1/worker/auth/login`；
- `GET /api/v1/worker/auth/me`；
- `POST /api/v1/worker/auth/logout`；
- `POST /api/v1/worker/auth/password`。

登录成功写入 Worker Principal；首次改密前只允许 `/auth/me`、`/auth/password`、`/auth/logout`。业务接口返回 `423 WORKER_PASSWORD_CHANGE_REQUIRED`。

### A2-4｜替换 Worker middleware

将 `auth.Worker` 从本地 Token 解析切换为真实 session 校验：

- 读取 Bearer Token；
- 查询 session 和 `employee_account`；
- 校验组织、状态、过期、撤销和密码版本；
- 将 Principal 写入 context；
- 401、423、403 使用统一错误结构；
- `local-worker-*` 仅在显式开发配置开启时可用，默认关闭，生产强制拒绝。

退出条件：不依赖前端时，使用真实 Token 能访问 `/auth/me`，首次改密状态能阻断业务接口。

---

## 8. A3｜Worker 业务接口和越权收口

### A3-1｜统一取当前师傅身份

改造所有 Worker Handler/Service，使业务参数只接收资源 ID、状态和业务字段，当前师傅 ID 统一从 Principal 获取。禁止继续使用：

- `worker_id` Query 参数；
- `assignee_id` Body 字段；
- `X-Worker-ID` 等身份 Header；
- 前端传入的“当前师傅”对象。

### A3-2｜逐路由补齐资源条件

至少覆盖：

- 工单列表和详情；
- 客户信息、地址、备注、补充资料和媒体；
- 接单、到达、开始服务、完工、返工、退回调度、改约；
- 图片、视频和现场凭证上传、读取、删除；
- 所有后续 Worker API。

每个查询/更新都要同时带：

```sql
org_id = principal.org_id
AND assignee_id = principal.subject_id
```

不属于当前师傅的资源返回 404，避免泄露资源存在性。

### A3-3｜越权与并发测试

- 师傅 A 访问师傅 B 的详情、媒体和操作接口；
- 篡改 URL、Query、Body、Header；
- 跨组织同 ID 工单访问；
- 并发改状态、重复上传和重复提交；
- 师傅禁用、密码重置后旧 Token 继续请求。

退出条件：所有 Worker 业务接口不再依赖本地身份，越权请求全部失败。

---

## 9. A4｜后台重置密码和师傅账号管理

### A4-1｜新增后台接口和权限

新增：

```text
POST /api/v1/admin/workers/{id}/reset-password
permission: worker.reset_password
```

后端事务：

1. 校验管理员会话、Casbin 权限和组织边界；
2. 锁定目标 Worker 账号；
3. 生成 `w+当前手机号` 的 Argon2id 哈希；
4. 递增 `password_version`，设置 `must_change_password=true`；
5. 撤销该师傅全部 session；
6. 写审计日志，不写明文密码；
7. 仅本次响应返回 `temporaryPassword`。

接口必须具备幂等/重复点击保护，不能因为重复请求留下多个有效旧 session。

### A4-2｜新增和编辑规则

- 新增师傅必须有合法且唯一手机号；
- 创建成功返回一次性 `initialPassword`；
- 修改手机号需要二次确认；
- 修改手机号后按新手机号重置密码、撤销 session 并要求改密；
- 师傅编号继续作为业务编号，不写入登录名；
- 无手机号师傅不能启用为可登录账号。

### A4-3｜React 页面

在师傅管理列表和详情增加：

- 登录手机号；
- 首次改密状态；
- 最近登录时间；
- “重置密码”按钮；
- 二次确认弹窗；
- 一次性密码展示、复制和关闭后不再显示的提示。

加载、无权限、冲突和服务器错误均显示中文可读提示，不直接展示 HTTP 401/403。

### A4-4｜审计和权限验收

- Casbin 菜单下新增 `worker.reset_password` 操作权限；
- 无该操作权限的管理员不能调用接口；
- 审计包含操作者、目标师傅、组织、时间、结果和请求追踪 ID；
- 审计内容不包含密码和完整 Token。

---

## 10. A5｜师傅微信小程序登录与首次改密

### A5-1｜新增页面和路由

在 `apps/wechat-worker-mini/miniprogram` 新增：

```text
pages/login/index
pages/change-password/index
```

未认证时不展示工作台、工单和我的 Tab；认证成功后恢复现有 Tab。

### A5-2｜实现请求和会话层

- Storage 使用 `fixpro.worker.accessToken`、`fixpro.worker.profile`；
- 所有请求自动添加 Bearer Token；
- 删除 `app.ts` 默认写入 `local-worker-1` 的行为；
- 401 清理 Token 并跳转登录；
- 423 跳转首次改密页；
- 登出清理所有 Worker 本地身份信息；
- 网络错误、认证失败和无权限错误显示中文提示。

### A5-3｜登录页

实现手机号、密码、登录按钮和友好错误态：

- 手机号使用数字键盘并校验 11 位；
- 密码支持显示/隐藏；
- 提交时防重复点击；
- 401 显示“手机号或密码错误，请重新输入”；
- 登录返回 `mustChangePassword=true` 时直接进入改密页；
- 登录成功且已改密时进入工作台。

### A5-4｜首次改密页

- 当前密码、新密码、确认新密码；
- 12 位、字母和数字校验；
- 两次密码不一致的表单内提示；
- 成功后清理旧 Token，提示后自动跳转登录页；
- 返回键、Tab 和页面重载都不能绕过 `mustChangePassword`。

### A5-5｜个人中心和工单回归

“我的”页面展示姓名、手机号、师傅编号，提供修改密码和退出登录。工作台、工单列表、详情和媒体均复用真实登录态，不显示本地演示师傅数据。

---

## 11. A6｜三端联调和正向链路

按真实 PostgreSQL 环境执行：

```text
后台新增师傅（手机号 13800138000）
→ 展示一次性初始密码 w13800138000
→ 启用师傅并派发工单
→ 师傅小程序使用手机号和初始密码登录
→ 被 423 拦截并完成首次改密
→ 使用新密码重新登录
→ 只看到自己的工单
→ 完成接单、上门、上传凭证和完工操作
→ 后台重置密码
→ 原 Token/旧密码失效
→ 师傅再次登录并再次改密
```

同时验证：

- 修改手机号后旧手机号不能登录，新手机号只能使用新初始密码登录；
- 禁用师傅后立即不能登录且旧 Token 失效；
- 后台权限变化不会影响已有管理员体系之外的 Worker 会话边界；
- 客户小程序、师傅小程序、管理后台三套登录 Storage/Cookie 互不污染。

---

## 12. A7｜安全、并发、性能与回归验收

### A7-1｜认证安全

- Argon2id 哈希参数符合项目安全基线；
- 登录错误不枚举账号；
- Token 随机、只存哈希、有过期和撤销；
- 密码、Token 不出现在日志、错误响应和审计中；
- 登录失败限流有效；
- 生产配置拒绝本地 Token。

### A7-2｜并发和幂等

- 同一师傅并发重置密码；
- 同一师傅并发修改密码；
- 登录与后台重置同时发生；
- 重复点击重置按钮；
- 过期 session 清理和查询并发；
- 工单状态操作的现有幂等规则不被破坏。

### A7-3｜越权

- A/B 师傅、跨组织工单、媒体、详情和状态变更；
- 伪造 `worker_id`、`assignee_id`、Header 和 URL；
- Worker 调用 admin API；
- 禁用、软删除和 DRAFT 账号绕过登录。

### A7-4｜性能

- `worker_session` 查询命中索引；
- 10 万订单下 Worker 列表仍只扫描当前师傅可见的工单范围；
- 不在每个请求中加载全量权限或全量师傅；
- 登录限流和 session 清理不阻塞业务请求。

### A7-5｜自动化命令

至少执行：

```text
go test ./...
go vet ./...
go test -race ./...
```

小程序执行现有 TypeScript 检查/构建命令，并在微信开发者工具中验证登录、改密和 Tab 路由。

退出条件：SPEC-010 第 13 节全部验收用例通过，并生成 `docs/test-reports/PLAN-010-e2e.md`。

---

## 13. A8｜上线收口、文档和回滚

### A8-1｜配置和文档

更新：

- `deploy_local.md`：新增 migration、启动后端、初始化师傅和登录验证步骤；
- Worker 小程序 README：配置 API 地址、首次登录和改密流程；
- 服务端配置说明：session TTL、登录限流、开发 Token 开关；
- API/OpenAPI 和错误码文档；
- 后台操作说明：初始密码和重置密码的一次性展示规则。

### A8-2｜生产收口

- 生产默认 `WORKER_DEV_TOKEN_ENABLED=false`；
- 删除或隔离 `local-worker-*` 自动注入代码；
- 检查构建产物、日志和测试账号不包含默认密码；
- 验证历史师傅的登录迁移结果；
- 监控登录失败、423 改密、密码重置、会话撤销和越权事件。

### A8-3｜回滚策略

- 代码回滚前不得把 Worker middleware 降级回生产本地 Token；
- migration 失败时停止发布并保留数据库备份；
- 认证字段只能前向兼容，不能通过 down migration 恢复明文密码；
- 如果前端发布滞后，后端保留接口兼容，不关闭真实登录接口。

---

## 14. 最终验收清单

- [x] `employee_account.username` 对 Worker 为手机号，师傅编号不再作为登录名。
- [x] 新增 ACTIVE 师傅可用 `w+手机号` 登录。
- [x] DRAFT、DISABLED、DELETED 师傅不可登录。
- [x] 首次登录业务接口返回 423 并进入改密流程。
- [x] 改密需要当前密码、新密码和确认密码，成功后重新登录。
- [x] 后台重置密码需要 `worker.reset_password` 权限和二次确认。
- [x] 重置响应只一次性返回 `temporaryPassword`，旧密码和旧 Token 失效。
- [x] 手机号变更会重置账号密码状态并撤销旧会话。
- [x] 师傅只能看到并操作自己的工单、客户信息和媒体。
- [x] 跨组织、篡改参数和调用后台接口均被拒绝。
- [x] 小程序无 Token、401、423、登出行为正确且提示为中文。
- [x] 生产配置默认关闭本地演示 Token。
- [ ] Go、React/小程序检查和真实 PostgreSQL E2E 全部通过（race 测试和微信开发者工具人工验收待补）。

---

## 15. 本次执行记录

### 已完成

- 新增 `000013_worker_auth`，建立 Worker 账号认证字段、`worker_session` 和 `worker.reset_password` 权限；
- 新增 `cmd/backfill-worker-auth`，已将本地 PostgreSQL 中历史 Worker 占位密码转换为 Argon2id，重复执行幂等；
- Go 后端新增手机号登录、当前身份、登出、修改密码和独立 Worker session；
- Worker 业务接口切换到真实 Principal，并保留组织 + `assignee_id` 服务端隔离；
- 后台师傅管理新增重置密码接口、Casbin 权限、二次确认和一次性密码展示；
- 师傅小程序新增登录页、首次改密页、Token 失效处理和个人中心账号信息；
- 更新 OpenAPI、`deploy_local.md` 和 SPEC-006 的认证说明；
- 在真实本地 PostgreSQL 上完成新增师傅、启用、初始登录、423 拦截、改密、重置密码、旧 Token 失效和越权详情访问验证。

### 验证结果

| 验证项 | 结果 |
| --- | --- |
| `go test ./...` | 通过 |
| `go vet ./...` | 通过 |
| 师傅小程序 `npm run typecheck` | 通过 |
| 管理后台 TypeScript 检查 | 通过（使用工作区临时 tsbuildinfo 路径） |
| 管理后台 `npm run lint` | 通过 |
| migration `go run ./cmd/migrate` | 本地 PostgreSQL 通过 |
| 存量密码回填重复执行 | 幂等通过 |
| `go test -race` | 未执行成功，当前环境缺少 CGO C 编译器 gcc |

### 待部署窗口完成

- 在微信开发者工具中导入 `apps/wechat-worker-mini`，人工确认登录页、首次改密页和 Tab 访问；
- 将服务端切换回 8080 并重启 Air；
- 生产部署前确认 `WORKER_DEV_TOKEN_ENABLED=false`，并完成发布环境 PostgreSQL 备份和迁移演练；
- 安装 gcc 后补跑 `go test -race ./...`。
