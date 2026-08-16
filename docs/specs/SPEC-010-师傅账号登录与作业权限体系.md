# SPEC-010｜师傅账号登录、首次改密与作业权限体系

**状态：** Draft for review  
**版本：** V1.0  
**日期：** 2026-08-16  
**适用工程：** `apps/server-go`、`apps/admin-web`、`apps/wechat-worker-mini`  
**关联 Spec：** SPEC-005 后台师傅管理、SPEC-006 师傅作业微信小程序、SPEC-009 后台用户角色与 Casbin 权限体系

---

## 1. 结论

师傅使用“手机号 + 密码”登录独立的师傅作业微信小程序。师傅账号复用 PostgreSQL 中的 `employee_account`，角色固定为 `WORKER`，登录名固定为手机号；后台管理员仍使用现有的管理员登录体系，不能与师傅账号混用。

师傅账号的正向链路为：

```text
后台新增/启用师傅
→ 系统以 w+手机号 生成初始密码
→ 师傅小程序使用手机号和初始密码登录
→ 首次登录强制修改密码
→ 修改成功后重新登录
→ 只能查看和操作分配给自己的工单
```

后台重置密码的链路为：

```text
后台师傅管理点击“重置密码”
→ 二次确认
→ 系统将密码重置为 w+当前手机号
→ 标记“首次登录需改密”并撤销该师傅现有登录态
→ 后台只展示一次初始密码
→ 师傅使用新密码登录并强制改密
```

本 Spec 解决账号认证、首次改密、后台重置密码和工单数据隔离；不包含师傅自主注册、微信手机号一键登录、短信验证码登录和多端账号合并。

---

## 2. 当前基线与问题

### 2.1 已有能力

- PostgreSQL 已有 `employee_account`，并已用于后台师傅档案；
- 师傅记录已有 `role=WORKER`、手机号、师傅编号、状态、工种和技能关联；
- Go 后端已经有 Worker API：
  - `GET /api/v1/worker/work-orders`
  - `GET /api/v1/worker/work-orders/{id}`
  - 工单接单、到达、完工、返工、媒体上传等操作接口；
- 当前工单查询和详情 SQL 已按 `assignee_id` 对师傅进行过滤；
- 管理后台已经接入管理员登录、Argon2id 密码哈希和 Casbin 权限体系；
- 师傅微信小程序已经存在工作台、工单和我的页面。

### 2.2 当前缺口

- Worker middleware 仍使用 `local-worker-{id}` 演示 Token，不能用于真实登录；
- 师傅小程序没有登录页面、登出和修改密码页面；
- `employee_account.password_hash` 当前是不可登录的占位值；
- 没有“首次登录必须修改密码”字段和状态；
- 后台师傅管理没有重置密码操作；
- 没有独立的师傅会话存储、过期和撤销机制；
- 需要将“只能查看自己的订单”落实到所有 Worker 详情、媒体和状态操作接口，而不是只依赖前端隐藏；
- 当前账号用户名可能是师傅编号，和本期“手机号即登录账号”的规则不一致。

---

## 3. 目标与成功标准

### 3.1 业务目标

1. 师傅可以使用手机号和初始密码进入作业小程序。
2. 师傅首次登录或后台重置密码后，必须先修改密码才能继续作业。
3. 师傅只能看到自己被派单的工单及这些工单的客户、预约、补充信息和履约信息。
4. 后台有明确的“重置密码”按钮、二次确认、一次性初始密码展示和操作审计。
5. 师傅账号按组织隔离，手机号在同一组织内唯一。
6. 账号禁用后立即不能登录，已有会话也不能继续调用业务接口。

### 3.2 验收成功标准

- 使用手机号和 `w+手机号` 能够登录新建且已启用的师傅账号；
- 首次登录只能进入改密页面，不能访问工单列表和工单详情；
- 新密码和确认密码不一致时不能提交；
- 改密成功后当前会话失效并跳转登录页，使用新密码可以再次登录；
- 后台重置密码需要二次确认，重置后旧密码和旧 Token 均失效；
- 重置后密码恢复为 `w+当前手机号`，并再次要求首次改密；
- 师傅 A 无法读取、修改或上传师傅 B 的任何工单数据；
- DRAFT、DISABLED、已删除师傅不能登录；
- 手机号在同一组织内重复时，新增和修改均返回明确错误；
- 生产环境不接受 `local-worker-*` 演示 Token；
- Go 单元/集成测试和小程序 TypeScript 检查通过。

---

## 4. 角色、边界与术语

| 角色 | 账号来源 | 可访问范围 | 密码管理 |
| --- | --- | --- | --- |
| 师傅（WORKER） | `employee_account` | 仅本人被分配的工单 | 首次登录自行修改；不能替其他师傅改密 |
| 后台管理员 | `admin_user` | 由 Casbin 菜单和操作权限决定 | 现有管理员登录体系 |
| 调度员/审核员 | `admin_user` 的角色 | 仍按后台角色权限控制 | 不使用师傅登录接口 |

约定：

- “手机号”是师傅登录账号，也是默认密码生成依据；
- “师傅编号”是业务展示编号，不作为登录账号；
- “初始密码”指系统按照 `w+手机号` 计算出的默认密码；
- “首次登录需改密”是账号状态，不等同于密码错误；
- 师傅小程序的 Token、Storage Key、页面路由均与客户小程序和管理后台隔离。

---

## 5. 账号模型与生命周期

### 5.1 账号主表

继续使用 `employee_account` 作为师傅账号主表，不新增一套重复的 `worker_user` 表。建议补充以下字段：

| 字段 | 类型 | 规则 |
| --- | --- | --- |
| `must_change_password` | `BOOLEAN NOT NULL DEFAULT TRUE` | 首次创建、后台重置密码、手机号变更后为 `TRUE` |
| `last_login_at` | `TIMESTAMPTZ NULL` | 最近一次登录成功时间 |
| `last_password_changed_at` | `TIMESTAMPTZ NULL` | 最近一次密码修改成功时间 |
| `password_version` | `INTEGER NOT NULL DEFAULT 1` | 密码变更时递增，用于使旧会话失效 |

已有的 `username` 字段在 Worker 账号中统一写入规范化手机号，后台展示仍可使用 `worker_no` 作为业务编号。迁移期间不得再把师傅编号写入 `username`。

### 5.2 手机号规则

- V1 按中国大陆手机号校验：`^1\\d{10}$`；
- 写入前去除首尾空格，不允许空格、短横线和其他格式；
- 同一 `org_id` 下手机号必须唯一；
- 不同组织可以存在相同手机号，但登录请求必须结合当前账号所属组织解析；
- 当前系统只有单组织登录入口时，默认使用配置的默认组织；多租户登录入口上线后需携带组织标识，不能跨组织猜测账号。

### 5.3 状态机

```text
DRAFT ──后台启用──> ACTIVE ──后台禁用──> DISABLED
  │                    │                    │
  └────不可登录─────────┴────可登录──────────┘

ACTIVE ──删除/软删除──> DELETED（不可登录）
```

- 只有 `ACTIVE` 师傅可以登录；
- DRAFT、DISABLED、DELETED 统一返回“账号不可用”，不泄露具体状态；
- 禁用或删除时撤销该师傅所有会话；
- 师傅可以存在未来预约工单，账号禁用规则沿用 SPEC-005，不因本 Spec 改变派单策略。

### 5.4 初始密码和重置规则

- 初始密码严格为小写 `w` 加当前手机号，例如手机号 `13800138000` 的初始密码为 `w13800138000`；
- 初始密码只保存 Argon2id 哈希，数据库、日志和接口响应中不得持久化明文；
- 新增师傅且手机号有效时，系统生成初始密码哈希并将 `must_change_password=true`；
- 后台重置密码时，按“当前手机号”重新生成 `w+手机号`，递增 `password_version`，设置 `must_change_password=true` 并撤销所有会话；
- 手机号变更后，必须重新生成新手机号对应的初始密码并要求改密，不能继续使用旧手机号或旧密码；
- 初始密码属于可预测密码，只作为 MVP 的临时密码。由于强制改密和会话撤销是硬性要求，生产后续应评估改为随机一次性密码或邀请链接。

---

## 6. 认证与会话设计

### 6.1 认证方式

Worker 使用独立的 Bearer access token，不复用管理员 Cookie、客户 Token 或本地演示 Token。

建议新增 `worker_session` 表保存会话：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | `BIGSERIAL` | 会话主键 |
| `org_id` | `BIGINT` | 组织 ID |
| `worker_id` | `BIGINT` | `employee_account.id` |
| `token_hash` | `BYTEA` | access token 的 SHA-256 哈希，唯一 |
| `password_version` | `INTEGER` | 创建会话时记录的密码版本 |
| `expires_at` | `TIMESTAMPTZ` | 默认 12 小时 |
| `last_seen_at` | `TIMESTAMPTZ` | 最近一次使用时间 |
| `revoked_at` | `TIMESTAMPTZ NULL` | 撤销时间 |
| `user_agent` | `TEXT NULL` | 诊断信息 |
| `ip` | `INET NULL` | 诊断信息 |
| `created_at` | `TIMESTAMPTZ` | 创建时间 |

实现要求：

- Token 使用密码学安全随机数生成，返回给客户端的仅是原始 Token；
- 数据库只保存 Token 哈希；
- 会话校验必须同时检查 `revoked_at IS NULL`、`expires_at > now()`、账号状态为 ACTIVE、会话密码版本等于账号 `password_version`；
- 密码修改、后台重置、禁用和删除均撤销旧会话；
- 生产环境禁止接受 `local-worker-*`。如保留本地联调能力，必须使用显式 `WORKER_DEV_TOKEN_ENABLED=true` 配置，默认关闭，并且不能在生产配置中开启。

### 6.2 Worker Principal

认证成功后写入请求上下文：

```text
orgID
subjectID = employee_account.id
role = WORKER
mobile
displayName
workerNo
mustChangePassword
passwordVersion
```

后续业务 Handler 只能从 Principal 取得 `orgID` 和 `subjectID`，不得信任客户端传入的 `worker_id`、`assignee_id` 或自定义身份 Header。

### 6.3 接口契约

#### 6.3.1 师傅登录

`POST /api/v1/worker/auth/login`

请求：

```json
{
  "mobile": "13800138000",
  "password": "w13800138000"
}
```

成功响应：

```json
{
  "token": "<opaque-access-token>",
  "expiresAt": "2026-08-17T10:00:00+08:00",
  "mustChangePassword": true,
  "worker": {
    "id": 12,
    "workerNo": "W000012",
    "mobile": "13800138000",
    "displayName": "张师傅"
  }
}
```

错误规则：

- 手机号格式错误：`400 WORKER_MOBILE_INVALID`；
- 账号不存在、密码错误、账号非 ACTIVE：统一 `401 WORKER_LOGIN_FAILED`，前端提示“手机号或密码错误，或账号暂不可用”；
- 不返回“手机号不存在”“账号已禁用”等可枚举账号状态的信息；
- 对同一 IP、手机号和设备组合做失败次数限制，具体阈值由实现阶段配置，至少要有日志和监控。

#### 6.3.2 当前身份

`GET /api/v1/worker/auth/me`

返回当前师傅基本资料、账号状态和 `mustChangePassword`，供小程序启动时恢复会话。

#### 6.3.3 登出

`POST /api/v1/worker/auth/logout`

撤销当前 Token，幂等返回 204 或统一成功响应。

#### 6.3.4 修改密码

`POST /api/v1/worker/auth/password`

请求：

```json
{
  "currentPassword": "w13800138000",
  "newPassword": "FixPro@20260816",
  "confirmPassword": "FixPro@20260816"
}
```

规则：

- `newPassword` 与 `confirmPassword` 必须完全一致；
- 最少 12 位，至少包含字母和数字；实现阶段可增加特殊字符要求，但不能降低 12 位下限；
- 新密码不能等于当前密码或 `w+手机号`；
- `currentPassword` 错误返回 `400 WORKER_CURRENT_PASSWORD_INVALID`；
- 校验通过后使用 Argon2id 重新生成哈希，递增 `password_version`，设置 `must_change_password=false`，撤销旧会话；
- 接口成功后客户端必须清理旧 Token 并跳转登录页重新登录，不在本地继续持有旧会话。

当账号 `must_change_password=true` 时，只允许访问 `GET /auth/me`、`POST /auth/password` 和 `POST /auth/logout`；访问工单业务接口统一返回 `423 WORKER_PASSWORD_CHANGE_REQUIRED`。

---

## 7. 工单数据隔离

### 7.1 后端强制规则

所有 `/api/v1/worker/**` 业务接口都必须从认证 Principal 获取当前师傅 ID，并在 SQL 中同时追加组织和师傅条件：

```sql
WHERE org_id = :principal_org_id
  AND assignee_id = :principal_worker_id
```

必须覆盖：

- 工单列表；
- 工单详情；
- 工单内客户姓名、手机号、地址、备注、补充信息和媒体；
- 接单、到达、开始服务、完工、返工、退回调度、改约等状态操作；
- 现场凭证、完工图片和视频的上传、读取、删除；
- 任何后续新增 Worker API。

### 7.2 API 约束

- 工单列表不接受客户端传入 `worker_id` 或 `assignee_id`；
- 工单详情 URL 中的 `{id}` 只表示目标工单，不表示身份；
- 不属于当前师傅的资源统一返回 404（避免泄露资源存在性），管理端越权操作仍按现有后台权限规范返回 403/404；
- 师傅不能调用 `/api/v1/admin/**`、管理员角色接口或 Casbin 管理接口；
- 媒体下载接口必须先校验媒体所属工单的 `assignee_id`，不能只校验媒体 ID。

### 7.3 组织隔离

- `org_id` 由认证会话确定，不接受客户端覆盖；
- 组织 A 的师傅不能读取组织 B 的同 ID 工单；
- 所有查询、更新、上传和状态流转测试都必须包含跨组织用例。

---

## 8. 后台师傅管理改造

### 8.1 列表和详情

“师傅管理”列表新增或明确展示：

- 师傅编号；
- 姓名；
- 登录手机号；
- 工种和技能；
- 师傅状态；
- 首次登录状态：`待首次改密` / `已完成改密`；
- 最近登录时间；
- 创建时间和更新时间；
- 操作：编辑、启用/禁用、重置密码。

### 8.2 新增和编辑

- 新增师傅必须填写有效且唯一的手机号；
- 创建成功后生成 `w+手机号` 的 Argon2id 哈希，`must_change_password=true`；
- 后端响应返回一次性 `initialPassword`，仅用于后台弹窗展示和复制，不写入列表、不写日志；
- 若因网络重试导致重复创建，使用手机号唯一约束和幂等/冲突响应，不能生成第二个账号；
- 编辑手机号时必须二次确认，保存后按新手机号重置初始密码、撤销会话并在详情中提示“需重新改密”；
- 师傅编号继续作为业务编号，不能作为登录名。

### 8.3 重置密码

新增接口：

`POST /api/v1/admin/workers/{id}/reset-password`

请求体可为空，必要时携带前端幂等键：

```json
{
  "confirm": true
}
```

处理规则：

1. 校验管理员会话、组织边界和 Casbin 操作权限；
2. 前端在发起请求前展示二次确认：“重置后该师傅现有登录将失效，并需要首次登录改密，是否继续？”；
3. 后端锁定目标账号，在事务中生成 `w+当前手机号` 的 Argon2id 哈希；
4. 递增 `password_version`，设置 `must_change_password=true`；
5. 撤销该师傅全部 Worker session；
6. 写入审计日志，记录操作者、目标师傅、组织、时间和结果，不记录明文密码；
7. 只在本次响应返回一次 `temporaryPassword`：

```json
{
  "workerId": 12,
  "temporaryPassword": "w13800138000",
  "mustChangePassword": true
}
```

前端展示成功弹窗，支持复制，并明确提示“请通过安全渠道交给师傅；关闭后不再显示”。如果响应丢失，管理员再次点击重置会生成同规则的密码并再次返回一次性结果。

### 8.4 权限和审计

新增独立操作权限：`worker.reset_password`，归属“师傅管理”菜单。只有具备该操作权限的后台用户可以重置密码；仅有 `worker.view` 不能重置，仅有 `worker.manage` 是否包含重置由权限初始化策略明确配置，不能依赖前端按钮隐藏。

审计事件建议：

- `worker.account_created`；
- `worker.password_reset`；
- `worker.password_changed`；
- `worker.login_success`；
- `worker.login_failed`；
- `worker.session_revoked`；
- `worker.account_disabled`。

---

## 9. 师傅微信小程序改造

### 9.1 页面和入口

新增页面：

| 页面 | 路径 | 说明 |
| --- | --- | --- |
| 登录 | `pages/login/index` | 手机号、密码、登录按钮、错误提示 |
| 首次改密 | `pages/change-password/index` | 当前密码、新密码、确认新密码 |

保留现有 Tab：

```text
工作台  |  工单  |  我的
```

未认证或 Token 失效时不能展示 Tab 页面，统一跳转登录页。登录页不显示业务 Tab。

### 9.2 登录交互

- 手机号输入框：数字键盘、11 位校验、去除首尾空格；
- 密码输入框：默认隐藏，可点击显示/隐藏；
- 登录按钮提交期间禁用，避免重复请求；
- 401 显示友好中文：“手机号或密码错误，请重新输入”；
- 账号不可用、网络错误和服务异常分别显示可理解的中文提示，不直接展示 HTTP 状态码或英文错误码；
- 登录成功且 `mustChangePassword=true` 时，直接跳转首次改密页；
- 登录成功且已完成改密时，进入工作台；
- 关闭小程序后重新打开，优先使用本地 Token 调用 `/auth/me` 恢复会话，失败则清除 Token 并回到登录页。

### 9.3 首次改密交互

- 页面标题为“首次登录，请设置新密码”；
- 显示安全提示：“为保障账号安全，请先修改初始密码”；
- 提供当前密码、新密码、确认新密码三个字段；
- 实时提示长度、字母和数字要求；
- 两次密码不一致时在表单内提示，不发起请求；
- 成功后提示“密码修改成功，请重新登录”，清理 Token 并自动跳转登录页；
- 用户不能通过返回、关闭弹窗或修改本地 Storage 绕过首次改密。

### 9.4 请求层和会话处理

- 使用独立 Storage Key，例如 `fixpro.worker.accessToken` 和 `fixpro.worker.profile`；
- 请求层自动附加 `Authorization: Bearer <token>`；
- 收到 401：清除 Token 和用户信息，跳转登录页；
- 收到 423 `WORKER_PASSWORD_CHANGE_REQUIRED`：跳转首次改密页；
- 收到 403/404：显示无权限或资源不存在，不暴露其他师傅信息；
- 不再在 `app.ts` 中默认写入 `local-worker-1`；
- 若保留本地联调开关，必须通过明确的开发配置打开，并在登录页标识为开发模式，生产构建不得携带。

### 9.5 个人中心

“我的”页面展示当前登录师傅的姓名、手机号、师傅编号和最近登录信息，提供：

- 修改密码；
- 退出登录。

个人中心不提供修改手机号、工种或技能入口，这些资料由后台维护。

---

## 10. 数据库迁移要求

新增迁移，建议命名：

```text
apps/server-go/db/migrations/000013_worker_auth.up.sql
apps/server-go/db/migrations/000013_worker_auth.down.sql
```

不得修改已执行的历史迁移。

### 10.1 表结构变更

- 为 `employee_account` 增加 `must_change_password`、`last_login_at`、`last_password_changed_at`、`password_version`；
- 确认并补充同组织手机号唯一索引；
- 新建 `worker_session`，包含 Token 哈希、组织、师傅、密码版本、过期和撤销字段；
- 为 session 增加 `token_hash` 唯一约束及 `(org_id, worker_id, revoked_at, expires_at)` 查询索引。

### 10.2 存量数据处理

- 对已有 `role=WORKER` 且手机号有效的账号，将密码哈希重置为 Argon2id(`w+手机号`)，设置 `must_change_password=true`；
- 对没有手机号或手机号不合法的师傅，不生成可登录密码，保持不可登录并在后台标记“请先完善手机号”；
- 不把任何明文初始密码写入迁移文件、日志或数据库；
- 回滚迁移不得恢复已删除的明文密码，只允许删除本次新增结构或按部署规范执行向前修复。

---

## 11. 安全要求

1. 密码必须使用项目已采用的官方 Argon2id 实现，禁止明文、MD5、SHA-1 或可逆加密。
2. 默认密码虽然是 `w+手机号`，但必须强制首次改密；后台重置后同样强制改密。
3. 登录错误统一响应，避免通过错误消息枚举手机号和账号状态。
4. Token 必须随机生成、数据库只保存哈希、设置过期时间并支持撤销。
5. 密码重置、修改密码、禁用账号时立即使旧会话失效。
6. 日志中不得出现密码、完整 Token、身份证号等敏感信息；手机号在普通日志中按需脱敏。
7. 所有 Worker 业务接口必须进行组织 + 师傅 ID 的服务端鉴权，前端控制不构成安全边界。
8. 管理后台的重置密码必须校验 Casbin 操作权限和组织边界，并写入审计日志。
9. 生产配置默认关闭本地演示 Token 和测试账号。

---

## 12. 接口错误码

| HTTP | 错误码 | 中文提示/用途 |
| --- | --- | --- |
| 400 | `WORKER_MOBILE_INVALID` | 手机号格式不正确 |
| 400 | `WORKER_PASSWORD_WEAK` | 新密码不符合安全要求 |
| 400 | `WORKER_PASSWORD_CONFIRM_MISMATCH` | 两次输入的新密码不一致 |
| 400 | `WORKER_CURRENT_PASSWORD_INVALID` | 当前密码错误 |
| 401 | `WORKER_LOGIN_FAILED` | 手机号或密码错误，或账号暂不可用 |
| 401 | `WORKER_SESSION_INVALID` | 登录已失效，请重新登录 |
| 403 | `WORKER_FORBIDDEN` | 无权访问该资源 |
| 404 | `WORKER_WORK_ORDER_NOT_FOUND` | 工单不存在或不属于当前师傅 |
| 409 | `WORKER_MOBILE_EXISTS` | 当前组织已有相同手机号的师傅 |
| 423 | `WORKER_PASSWORD_CHANGE_REQUIRED` | 请先修改初始密码 |
| 429 | `WORKER_LOGIN_RATE_LIMITED` | 登录尝试过于频繁，请稍后再试 |

管理后台重置密码相关错误必须使用现有后台错误格式，并补充明确的 Casbin 无权限提示。

---

## 13. 测试与验收用例

### 13.1 后端认证

- 新增 ACTIVE 师傅后，手机号 + `w+手机号` 登录成功，返回 `mustChangePassword=true`；
- DRAFT、DISABLED、DELETED 师傅登录失败，且不泄露具体状态；
- 手机号不存在、密码错误的响应状态和错误码一致；
- 登录成功更新 `last_login_at`；
- Token 过期、撤销、密码版本不一致时业务接口均返回 401；
- 生产配置拒绝 `local-worker-*`。

### 13.2 首次改密

- 首次登录访问工单列表返回 423；
- 当前密码错误不能修改；
- 新密码和确认密码不一致不能修改；
- 新密码不满足 12 位、字母和数字要求不能修改；
- 修改成功后旧 Token 和旧密码失效，新密码可登录；
- 修改后 `must_change_password=false`、`password_version` 递增。

### 13.3 后台重置

- 没有 `worker.reset_password` 权限的用户不能调用重置接口；
- 重置前端必须二次确认；
- 重置后返回一次性 `temporaryPassword=w+手机号`；
- 重置后旧密码、旧 Token 均失效；
- 师傅再次登录后被强制改密；
- 审计记录不包含明文密码。

### 13.4 数据隔离

- 师傅 A 列表只能返回 A 的工单；
- A 访问 B 的详情、媒体、状态操作、改约和完工接口均失败；
- 组织 A 的师傅不能访问组织 B 的同 ID 资源；
- 通过篡改 URL、Query、Body 或 Header 均不能改变当前师傅身份。

### 13.5 小程序

- 无 Token 启动进入登录页；
- 登录失败显示中文友好提示，不出现 401；
- 首次登录进入改密页，不能通过 Tab 或返回绕过；
- 改密成功自动回登录页；
- Token 过期自动清理并回登录页；
- 工单列表、详情、客户信息和履约操作都来自当前登录师傅；
- 退出登录后返回登录页且本地 Token 被清除。

---

## 14. 实施顺序（仅作为开发边界，不替代 PLAN）

1. 新增数据库字段、`worker_session` 和存量师傅密码迁移；
2. 在 Go 后端实现 Worker 登录、会话中间件、`me`、登出和改密接口；
3. 将所有 Worker 业务接口切换到真实 Principal，并补齐组织 + `assignee_id` 鉴权；
4. 后台师傅管理增加重置密码接口、权限点、二次确认和一次性密码展示；
5. 师傅小程序增加登录和首次改密页面，接入 Token 生命周期；
6. 完成后端并发、越权、会话撤销、密码安全和小程序端到端验收；
7. 关闭生产环境本地演示 Token。

---

## 15. 明确不纳入本期

- 师傅自主注册或自行找回密码；
- 短信验证码、微信手机号授权登录；
- 一个手机号绑定多个组织并在登录页选择组织；
- 师傅修改手机号、姓名、工种、技能和证书；
- 管理员查看师傅密码或导出密码；
- 客户小程序和师傅小程序共享登录态；
- 随机临时密码、短信发送和邀请链接（可作为后续安全增强）。

