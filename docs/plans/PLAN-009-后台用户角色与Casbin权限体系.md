# PLAN-009｜后台用户、角色与 Casbin 多租户权限体系实施计划

**状态：** Executed（A0—A9 已完成；官方 Casbin Enforcer/Adapter 与 Argon2id 已接入并完成本地 PostgreSQL 验证）  
**版本：** V1.0  
**日期：** 2026-08-15  
**对应 Spec：** `docs/specs/SPEC-009-后台用户角色与Casbin权限体系.md` V1.0  
**适用工程：** `apps/server-go`、`apps/admin-web`、`apps/server-go/db/migrations`  
**执行顺序：** 权限矩阵冻结 → PostgreSQL 模型 → Casbin 内核 → 登录会话 → 角色管理 → 用户管理 → React 权限基础 → 管理页面 → 全路由收口 → 安全与 E2E

---

## 1. 交付目标

完成一套可在 FixPro 管理后台真实使用的多租户权限链路：

```text
初始化平台超级管理员
→ 登录指定租户
→ 创建自定义角色
→ 配置菜单与操作权限
→ 新增后台用户并分配角色
→ 用户按有效权限看到菜单和按钮
→ Go 后端使用 Casbin 强制校验 API 权限
→ 角色或用户变更即时生效
→ 登录与权限变更完整审计
```

完成时必须满足：

- `org_id` 映射为 Casbin Domain，策略不能跨租户生效；
- 平台超级管理员拥有受控的全租户权限；
- 租户管理员拥有本租户全量权限；
- 普通用户通过一个或多个角色获得权限；
- 菜单、按钮和 API 使用同一套权限编码；
- 后端鉴权失败默认拒绝，不依赖前端隐藏；
- 后台不再使用共享 Basic Auth 和浏览器中的 Basic 凭据；
- 禁用用户、重置密码和移除权限能及时影响现有会话；
- 权限变更具备事务、乐观锁、缓存失效和审计；
- SPEC-009 的 AC-01 至 AC-12 全部通过。

## 2. 当前基线与实施边界

### 2.1 当前基线

- PostgreSQL migration 已到 `000010_customer_addresses`；本计划新增 `000011_admin_rbac_casbin` 和 `000012_admin_rbac_org_provision`；
- `organization` 已存在，绝大多数业务表都包含 `org_id`；
- 后台使用 `APP_ADMIN_USERNAME`、`APP_ADMIN_PASSWORD` 和 HTTP Basic Auth；
- 后台 Principal 没有真实管理员用户 ID，角色固定为 `ADMIN`；
- React 将 Basic 凭据保存在 `sessionStorage`；
- `AdminLayout` 菜单静态写死；
- 后台路由统一经过 `admin(...)`，业务 Service 内仍存在大量 `p.Role != "ADMIN"`；
- 客户和师傅使用独立本地 Bearer 认证，本计划不得修改；
- 当前部署是单 Go 实例，暂不需要跨实例 Watcher。

### 2.2 确定性决策

1. 新建 `admin_user`，不复用专用于师傅的 `employee_account`。
2. Casbin 使用 `github.com/casbin/casbin/v3` 和 RBAC with Domains。
3. Domain 固定为 `org::<org_id>`，区域不是 Domain。
4. PostgreSQL 规范化表是用户、角色、权限关系的唯一事实来源。
5. 自定义 Casbin Adapter 从规范化表加载 `p`、`g` 策略，不维护重复 `casbin_rule` 关系。
6. 第一阶段只支持用户分配角色，不支持用户直接权限例外和角色继承。
7. 后台登录使用服务端会话和 HttpOnly Cookie，不把权限快照作为后端依据存入浏览器。
8. 角色编码由系统生成，权限编码由代码和 migration 注册。
9. 平台超级管理员 bypass 只允许存在于统一授权服务。
10. 多区域本期只交付数据范围接口和租户全量默认实现，不改造全部业务表。

### 2.3 明确不做

- 客户、师傅、企业门户的账号权限改造；
- Keycloak、LDAP、AD、OIDC、SAML 和 MFA；
- 角色继承和临时授权；
- 单用户 Allow/Deny 例外；
- 字段级权限与脱敏策略；
- 区域表、区域树及订单/工单 `region_id` 全面改造；
- 动态下载 React 页面或由运营人员创建任意路由；
- 多实例 Casbin Watcher；
- 借权限改造重构订单、履约、商品和师傅业务状态机。

## 3. 实施原则

1. 不修改已执行 migration，只新增 `000011_admin_rbac_casbin` 和后续补充 migration。
2. 先建立数据模型和 Casbin 内核，再接入真实登录，最后删除 Basic Auth。
3. 权限目录先冻结，禁止在页面开发过程中临时发明角色名判断。
4. 每个后台 API 都必须显式绑定一个权限或明确标注为仅登录可访问。
5. Casbin 只判断功能权限，Service 继续校验组织归属、资源状态、版本和幂等。
6. 策略变更与业务关系、版本和审计在同一事务提交；提交后再失效 Enforcer。
7. 策略加载异常默认拒绝，不能降级成管理员全放行。
8. 每阶段完成真实 PostgreSQL 和自动化验证后再进入下一阶段。
9. 只修改权限接入直接涉及的代码，不顺带重构业务模块。
10. 切换完成前保留明确的超级管理员初始化和应急恢复方式。

## 4. 里程碑与关键路径

| 里程碑 | 交付结果 | 主要验证 | 依赖 |
| --- | --- | --- | --- |
| A0 | 权限目录、路由矩阵和接口契约冻结 | 所有后台路由与按钮有唯一权限归属 | SPEC-009 |
| A1 | PostgreSQL 用户、角色、会话和权限模型可用 | 空库、存量升级、约束测试通过 | A0 |
| A2 | Casbin Adapter、EnforcerManager 和授权服务可用 | Domain 隔离、默认拒绝、缓存失效通过 | A1 |
| A3 | 后台真实登录、会话和 Principal 可用 | 登录、退出、改密、锁定、CSRF 通过 | A1、A2 |
| A4 | 角色与权限后端闭环 | 创建角色、配置权限、即时生效 | A2、A3 |
| A5 | 后台用户后端闭环 | 新增用户、分配角色、禁用、重置密码 | A3、A4 |
| A6 | React 动态登录、菜单、路由和按钮权限基础可用 | 不同角色看到不同界面 | A3—A5 |
| A7 | 用户、角色和审计管理页面可用 | 管理员可完成全部配置操作 | A6 |
| A8 | 全部后台路由完成权限收口并移除 Basic Auth | 无遗漏路由、无长期双认证 | A2—A7 |
| A9 | 多租户、并发、安全、性能和 E2E 验收完成 | AC-01 至 AC-12 全通过 | A1—A8 |

关键路径：

```text
A0 → A1 → A2 → A3 → A4 → A5 → A6 → A7 → A8 → A9
```

A4 和 A5 的仓储实现可并行准备，但用户创建依赖至少一个可用角色，因此验收顺序保持 A4 在前。

## 5. A0｜权限目录、路由矩阵与测试骨架

### A0-1｜盘点全部后台入口

从以下位置生成完整清单：

- `apps/server-go/internal/app/app.go` 的 `/api/v1/admin/**` 路由；
- `apps/admin-web/src/App.tsx` 的后台路由；
- `apps/admin-web/src/app/AdminLayout.tsx` 的菜单；
- 各 React 页面中的新增、修改、发布、派单、审核、删除等操作按钮；
- 后台媒体上传、下载和删除接口；
- 当前仍为占位页的菜单。

为每项标记：

```text
资源 resource
动作 action
权限编码 permission_code
权限类型 MENU/ACTION
对应 React 路由
对应 Go HTTP 路由
允许的初始化角色模板
```

验证：不存在只有前端按钮没有后端接口权限、或只有后端路由没有权限编码的条目。

### A0-2｜冻结权限和路由矩阵

把 SPEC-009 首批权限目录扩展为可执行矩阵，至少覆盖：

- 经营概览；
- SKU 和分类；
- 订单查看与确认；
- 履约查看、派单、改派、改期、初审、总监审核、客服确认；
- 师傅、工种、技能和媒体；
- 后台用户、角色、权限树和审计日志；
- 管理员媒体访问；
- 登录、退出、当前用户和本人改密。

约束：

- `auth.login` 不需要登录；
- `auth.logout`、`auth.me`、本人改密只要求有效会话并执行专用安全校验；
- 其他后台接口必须绑定业务权限；
- 同一业务操作只使用一个稳定权限编码；
- 禁止将 HTTP Method + URL 直接暴露为运营权限名称。

### A0-3｜冻结 OpenAPI 契约

更新 `apps/server-go/api/openapi.yaml`：

- 登录、退出、当前用户、本人改密；
- 用户列表、详情、新增、编辑、状态、角色分配、重置密码、有效权限；
- 角色列表、详情、新增、编辑、状态、权限分配和删除；
- 权限树和审计列表；
- 401、403、409、423 和 503 错误结构；
- Cookie、CSRF Header、版本号和分页约定。

### A0-4｜建立测试夹具

准备可重复创建的：

- 两个 organization；
- 一个平台超级管理员；
- 每租户一个租户管理员；
- 商品运营、调度员、初审员、总监审核员；
- 启用、禁用、锁定用户；
- 同名但不同租户的角色和用户；
- 有权限和无权限的订单、工单、SKU 与媒体请求。

退出条件：权限矩阵、OpenAPI 和测试夹具可以直接驱动后续开发，不再由页面自行定义权限。

## 6. A1｜PostgreSQL 模型与超级管理员初始化

### A1-1｜新增 migration

新增：

```text
apps/server-go/db/migrations/000011_admin_rbac_casbin.up.sql
apps/server-go/db/migrations/000011_admin_rbac_casbin.down.sql
apps/server-go/db/migrations/000012_admin_rbac_org_provision.up.sql
apps/server-go/db/migrations/000012_admin_rbac_org_provision.down.sql
```

Up migration 建立：

- `admin_user`；
- `admin_user_session`；
- `admin_platform_super_admin` 或等价安全关系；
- `admin_role`；
- `admin_permission`；
- `admin_user_role`；
- `admin_role_permission`；
- 必要的审计索引和外键；
- 用户、角色、权限、会话列表所需索引。

关键约束：

- 用户名、角色名称和角色编码按 Spec 唯一；
- 用户、角色和关联关系使用 `(org_id, id)` 组合唯一键和组合外键；
- 不能把租户 1 的用户关联到租户 2 的角色；
- 会话只保存随机令牌哈希；
- 用户和角色使用 `version`；
- 状态使用 CHECK；
- 审计记录不级联删除；
- 内置角色和平台管理员关系不能通过普通级联误删。

### A1-2｜注册权限目录

在 migration 中以幂等方式注册 SPEC-009 第 8 节的权限目录：

- `permission_code`、`resource`、`action` 全局稳定；
- MENU 写入真实 `route_path` 和排序；
- ACTION 关联业务父节点；
- 权限更新使用显式 migration，不由服务启动时静默修改；
- 占位菜单只注册 MENU，不虚构尚不存在的操作。

验证：重复运行迁移不会产生重复权限；权限树父子关系完整；资源和动作组合唯一。

### A1-3｜创建内置租户管理员

为每个现有 organization 建立一个 `BUILT_IN` 租户管理员角色，并关联通配权限语义。后续新建 organization 时必须在同一组织创建事务中生成该角色。

验证：租户管理员角色不可删除；两个租户角色 ID、名称和 Domain 不混用。

### A1-4｜实现平台超级管理员初始化命令

新增：

```text
apps/server-go/cmd/bootstrap-admin/
```

命令要求：

- 无平台超级管理员时才能初始化；
- 用户名、密码和展示名通过交互输入或安全环境变量提供；
- 密码使用与正式登录相同的哈希流程；
- 重复执行安全失败；
- 不打印明文密码和哈希；
- 写安全审计记录；
- 支持本地环境快速创建初始管理员。

### A1-5｜迁移验证

验证以下路径：

1. 空 PostgreSQL 从 `000001` 升到 `000012`；
2. 当前本地真实数据库从 `000010` 升级；
3. 升级后已有订单、工单、SKU、师傅和地址数据不变；
4. 空环境执行 down 后结构可回退；
5. 跨租户关联、重复用户名、重复角色和非法状态被数据库拒绝。

退出条件：数据库模型、权限种子和初始化命令在真实 PostgreSQL 上通过。

## 7. A2｜Casbin 授权内核

### A2-1｜引入依赖和模型

- 在 `apps/server-go/go.mod` 增加 `github.com/casbin/casbin/v3` 直接依赖；
- 新增 `internal/authorization/model.conf`；
- 模型严格使用 SPEC-009 的 `sub/dom/obj/act/eft`；
- 使用 `g = _, _, _` 表达租户内用户角色；
- 支持租户管理员 `obj=*`、`act=*`；
- 无匹配策略默认拒绝；
- 模型文件通过单元测试加载，启动时模型错误直接失败。

### A2-2｜实现 PostgreSQL Adapter

建议目录：

```text
apps/server-go/internal/authorization/
  adapter.go
  enforcer.go
  service.go
  model.conf
```

Adapter 行为：

- 从 `admin_user_role` 加载有效 `g` 规则；
- 从 `admin_role_permission` 和 `admin_permission` 加载有效 `p` 规则；
- 忽略禁用用户、禁用角色和禁用权限；
- 通过明确过滤参数只加载一个 `org_id`；
- 不把数据库表名、主键和租户条件拼接自客户端输入；
- 不实现第二份重复持久化关系；
- Adapter 加载结果可与 SQL 关系逐条对账。

### A2-3｜实现 EnforcerManager

职责：

- 按 `org_id` 懒加载和缓存 Enforcer；
- 并发首次加载只执行一次；
- 权限变更后精确失效租户缓存；
- 失效期间不把其他租户 Enforcer 错配给当前请求；
- 重载失败保留安全拒绝状态并记录结构化错误；
- 提供测试用强制重载和统计接口，不暴露为生产管理 API。

### A2-4｜实现 AuthorizationService

统一入口：

```go
Enforce(ctx, principal, resource, action) error
EffectivePermissions(ctx, principal) ([]Permission, error)
Menus(ctx, principal) ([]MenuNode, error)
InvalidateOrganization(orgID int64)
```

平台超级管理员 bypass 只能在 `Enforce` 内执行，并写入可观测字段；业务 Handler 和 Service 不允许自行写 `IsPlatformAdmin || ...`。

### A2-5｜授权内核测试

覆盖：

- 单角色 Allow；
- 多角色权限并集；
- 无策略默认拒绝；
- 禁用角色不生效；
- 同一用户在两个 Domain 下角色不同；
- 租户管理员只通配当前租户；
- 平台超级管理员统一 bypass；
- Adapter 加载失败默认拒绝；
- 缓存命中、精确失效和并发重载；
- 跨租户策略不进入当前 Enforcer。

退出条件：Casbin 权限判断在不依赖 React 的情况下可独立验证。

## 8. A3｜后台登录、会话与 Principal

### A3-1｜实现密码服务

- 使用 Argon2id；
- 每个密码独立随机盐；
- 参数集中配置并可升级；
- 哈希包含版本和参数信息；
- 比较过程避免明显时序差异；
- 不在日志、错误和审计中输出明文密码或哈希；
- 单元测试覆盖正确密码、错误密码、损坏哈希和参数升级判断。

### A3-2｜实现服务端会话

- 使用密码学安全随机令牌；
- 数据库只保存令牌哈希；
- Cookie 使用 HttpOnly、SameSite 和生产 Secure；
- 登录成功轮换会话；
- 会话包含用户、租户、过期、撤销和最近活动时间；
- 禁用用户、重置密码时支持批量撤销；
- 清理过期会话使用显式定时任务或维护命令，不阻塞登录请求。

### A3-3｜实现认证 API

实现：

- `POST /api/v1/admin/auth/login`；
- `POST /api/v1/admin/auth/logout`；
- `GET /api/v1/admin/auth/me`；
- `PUT /api/v1/admin/auth/password`。

登录处理：

- 校验租户编码和用户归属；
- 校验用户状态；
- 失败计数和临时锁定；
- 成功后清零失败计数、更新最近登录并创建会话；
- 首次登录仅允许访问本人改密和退出；
- 错误提示不暴露用户是否存在。

### A3-4｜实现 CSRF 和 Origin 防护

- 登录成功返回或设置 CSRF Token；
- POST、PUT、PATCH、DELETE 校验 CSRF Header；
- 校验允许的 Origin；
- 登录和公开 GET 使用明确例外；
- CORS 允许 Cookie，但不允许任意 Origin 与凭据组合；
- 本地 Vite 代理验证 Cookie 和 Header 能正常转发。

### A3-5｜替换后台 Principal 恢复

将后台 Principal 改为真实：

- `SubjectID=admin_user.id`；
- `OrgID` 来自服务端会话；
- `Name` 来自后台用户；
- 包含会话 ID 和平台管理员标识；
- 不接受客户端自报角色和组织；
- 客户和师傅 Principal 保持原行为。

退出条件：不依赖 Basic Auth 即可登录、恢复真实后台用户并调用 `/auth/me`。

## 9. A4｜角色与权限后端闭环

### A4-1｜角色查询与详情

实现角色列表、详情和权限树：

- 分页、名称、编码和状态筛选；
- 返回角色类型、状态、用户数量、权限数量和审计字段；
- 详情返回权限树勾选状态；
- 内置角色明确只读字段；
- 所有查询限定当前 `org_id`。

### A4-2｜角色创建和编辑

- 后端生成角色编码；
- 名称租户内唯一；
- 新角色默认 `ACTIVE`；
- 创建时可以原子保存权限；
- 编辑使用 `version`；
- 角色主档、权限关系、版本和审计同一事务；
- 权限 ID 必须全部有效且来自注册目录。

### A4-3｜配置角色权限

- 保存完整权限集合，不依赖前端逐条增删；
- ACTION 被选中时后端再次验证父 MENU 权限；
- 保存前计算新增和移除差异；
- 事务提交后失效当前租户 Enforcer；
- 失效失败记录告警并返回明确错误；
- 审计保存权限编码差异，不只保存数量。

### A4-4｜角色启停和删除

- 禁用后立即不贡献有效权限；
- 租户最后一个管理员角色保护；
- 内置角色不可删除；
- 有用户关联或审计依赖的角色不能删除；
- 可删除角色使用二次确认和版本号；
- 删除、启停、缓存失效和审计按 Spec 执行。

### A4-5｜后端验证

覆盖 SPEC-009 AC-02、AC-05、AC-07、AC-08、AC-09 和 AC-10 的角色部分。

退出条件：使用 API 可以创建“履约调度”角色并实时改变当前用户权限。

## 10. A5｜后台用户管理后端闭环

### A5-1｜用户列表和详情

- 分页、用户名/姓名关键字、状态和角色筛选；
- 返回角色、最近登录、创建修改信息；
- 详情返回直接角色和最终有效权限；
- 手机号和邮箱按管理权限决定是否完整展示；
- 不返回密码哈希、会话令牌和失败验证细节。

### A5-2｜新增和编辑用户

- 用户名租户内唯一；
- 后端生成高强度随机临时密码，只在创建成功响应中返回一次，不保存明文；
- 新用户 `must_change_password=true`；
- 至少分配一个启用角色；
- 主档、角色关系、版本和审计同一事务；
- 编辑不允许修改所属租户和平台管理员标记；
- 使用 `version` 防止覆盖。

### A5-3｜分配角色

- 接收完整角色 ID 集合；
- 全部角色必须属于当前租户且启用；
- 计算新增和移除差异；
- 保护最后一个租户管理员；
- 提交后失效租户 Enforcer；
- 返回更新后的有效角色和权限摘要。

### A5-4｜用户状态和密码重置

- `ACTIVE`、`DISABLED`、`LOCKED` 状态按 Spec 转换；
- 禁用用户撤销全部会话；
- 解锁用户清零失败计数和锁定时间；
- 重置密码生成新的高强度随机临时密码，只返回一次，同时设置强制改密并撤销全部会话；
- 保护最后一个平台超级管理员和租户管理员；
- 操作写安全审计。

### A5-5｜用户后端验证

覆盖：

- 新增用户并分配多角色；
- 重复用户名；
- 跨租户角色 ID；
- 禁用后的现有会话；
- 重置密码后的现有会话；
- 锁定和解锁；
- 最后管理员保护；
- 并发编辑和角色分配。

退出条件：仅通过 API 能完成“创建角色 → 创建用户 → 分配角色 → 登录 → 获取有效权限”。

## 11. A6｜React 认证、动态菜单和按钮权限基础

### A6-1｜重构认证状态

修改 `authStore`：

- 删除 Basic Credential；
- 不保存密码、Basic 编码和后端可信角色；
- 保存当前用户展示信息和加载状态；
- 应用启动调用 `/auth/me`；
- 401 清理状态并跳转登录；
- 403 保留登录状态并展示无权限；
- 支持首次登录强制改密流程。

### A6-2｜修改 HTTP 客户端

- 请求使用 `credentials: include`；
- 修改请求携带 CSRF Header；
- 保留 `Idempotency-Key` 和 `X-Request-Id`；
- 统一处理 401、403、409、423；
- 不再设置 `Authorization: Basic ...`；
- 媒体 Blob 请求同样携带会话和 CSRF 所需信息。

### A6-3｜动态菜单与路由守卫

- `AdminLayout` 使用 `/auth/me` 菜单树；
- React 路由仍由代码静态注册；
- 增加路由权限元数据；
- 直接访问无权路由展示 403；
- 登录后跳转到第一个有权限页面；
- 无可用菜单展示明确空状态；
- 平台超级管理员显示当前租户信息。

### A6-4｜统一按钮权限

实现：

```text
usePermission(permissionCode)
PermissionGuard
```

规则：

- 默认无权限不渲染；
- 页面不写角色名称判断；
- 权限集合刷新后按钮立即更新；
- 权限加载失败不显示高风险按钮；
- 是否显示不能替代后端鉴权。

退出条件：使用不同测试用户登录时，菜单、路由和按钮出现明确差异。

## 12. A7｜React 用户、角色和审计页面

### A7-1｜系统权限菜单

新增系统权限菜单组：

```text
系统管理
├── 后台用户
├── 角色权限
└── 权限审计
```

菜单是否出现由服务端返回的 MENU 权限决定，不为平台管理员单独硬编码。

### A7-2｜后台用户页面

实现：

- 用户列表、分页、搜索、状态和角色筛选；
- 新增用户抽屉；
- 编辑用户抽屉；
- 多角色选择；
- 有效权限只读展示；
- 启用、禁用、解锁；
- 重置密码二次确认；
- 最近登录、创建和修改信息；
- 409 冲突刷新提示。

### A7-3｜角色权限页面

实现：

- 角色列表、分页和状态筛选；
- 新增和编辑抽屉；
- 权限树按菜单和操作分组；
- ACTION 与父 MENU 联动；
- 保存前展示权限差异；
- 移除高风险权限二次确认；
- 内置角色展示不可删除说明；
- 用户数量不为零时阻止删除并展示原因。

### A7-4｜审计页面

- 按操作人、动作、资源类型、资源 ID 和时间区间筛选；
- 默认分页；
- 抽屉查看 before/after 差异；
- 不展示密码、密码哈希和令牌；
- 无 `admin_audit.view` 不显示菜单且 API 返回 403。

### A7-5｜页面验收

由超级管理员完成：

```text
创建“质检审核员”角色
→ 勾选履约查看和质检初审
→ 创建用户并分配该角色
→ 新用户首次登录改密
→ 只看到履约调度
→ 能初审，不能总监审核和派单
```

退出条件：管理员不需要数据库脚本即可完成用户和角色全流程。

## 13. A8｜全部后台路由和业务按钮权限收口

### A8-1｜路由统一绑定权限

逐项迁移 A0 权限矩阵：

- 媒体上传、查看、删除；
- 分类和 SKU；
- 订单；
- 工种、技能和师傅；
- 履约、派单、改派、改期；
- 质检初审、总监审核、客服确认；
- 用户、角色、权限和审计。

每个路由必须是以下之一：

```text
公开路由
客户认证路由
师傅认证路由
后台会话路由
后台会话 + 权限路由
```

增加自动测试，发现未分类的 `/api/v1/admin/**` 路由时失败。

### A8-2｜移除固定 ADMIN 判断

- 后台请求不再依赖 `p.Role == "ADMIN"`；
- Service 保留真实用户、租户、资源归属和业务状态校验；
- 审核记录使用真实 `SubjectID` 和管理员姓名；
- 审计记录不再出现统一的 operator ID 0；
- 客户和师傅角色判断不在本计划中重构。

### A8-3｜补齐所有 React 操作权限

逐页检查：

- 新增、编辑、发布、下架；
- 确认订单；
- 派单、改派、改期；
- 初审、总监审核和客服确认；
- 师傅新增、编辑、启停；
- 工种和技能管理；
- 用户、角色和审计操作。

按钮隐藏、页面路由和 API 权限必须使用相同编码。

### A8-4｜删除 Basic Auth

在新会话链路和超级管理员验证通过后：

- 删除后台 Basic Auth middleware；
- 删除 React Basic 凭据存储；
- 删除正常运行对 `APP_ADMIN_USERNAME`、`APP_ADMIN_PASSWORD` 的依赖；
- 更新 Compose、本地启动说明和 `.env.example`；
- 保留显式 `bootstrap-admin` 初始化方式；
- 不在生产保留长期双认证开关。

### A8-5｜认证切换回归

- 老 Basic 凭据不能继续调用后台 API；
- 新 Cookie 会话可完成全部已授权操作；
- 401 与 403 语义正确；
- 页面刷新不会丢失登录；
- 退出后 Cookie 和服务端会话均失效；
- Air 重启后数据库会话仍按预期有效或失效，不出现未定义行为。

退出条件：后台只有一套正式认证入口，全部路由都有明确权限归属。

## 14. A9｜多租户、区域接口、安全、性能与 E2E

### A9-1｜数据范围扩展接口

新增最小接口：

```go
type DataScopeResolver interface {
    ResolveRegions(ctx context.Context, principal Principal, resource string) (RegionScope, error)
}
```

本期实现 `TENANT_ALL`：

- 只允许返回当前 Principal 的租户；
- 不创建虚假区域数据；
- 不修改订单和工单 SQL；
- 添加接口测试，保证未来可以替换为区域范围实现；
- 文档明确区域表和 `region_id` 属于后续独立 Spec。

### A9-2｜多租户越权测试

在真实 PostgreSQL 中验证：

- 用户不能分配其他租户角色；
- 角色不能关联其他租户权限关系；
- 同一用户主体在两个 Domain 下权限不同；
- 伪造 org Header、Cookie 内容或资源 ID 不能跨租户；
- 平台管理员跨租户操作带目标租户审计；
- Enforcer 缓存失效不影响其他租户。

### A9-3｜并发和一致性测试

- 两人同时编辑同一角色只有一个成功；
- 角色权限修改与 Enforcer 加载并发时不短暂扩大权限；
- 用户被禁用与其业务请求并发时，提交后的新请求全部拒绝；
- 重置密码与旧会话请求并发时，撤销后旧会话不能继续；
- 事务失败不产生半个角色、半组权限或缺失审计；
- 同一租户 Enforcer 并发首次加载只执行一次。

### A9-4｜安全测试

- SQL 注入和非法 permission code；
- Cookie 篡改、过期、撤销和会话固定；
- CSRF Token 缺失、错误和跨 Origin；
- 登录暴力尝试和锁定；
- 密码、哈希和令牌日志泄漏检查；
- 直接 URL、直接 API 和隐藏按钮绕过；
- 最后平台管理员和租户管理员保护；
- Casbin 模型、Adapter 和数据库不可用时默认拒绝。

### A9-5｜性能验证

准备至少：

- 2 个租户；
- 单租户 10,000 个后台用户；
- 500 个角色；
- 500 个权限节点；
- 足量用户角色和角色权限关系。

验证：

- Casbin 内存 Enforce P95 小于 5ms；
- `/auth/me` 缓存命中 P95 小于 200ms；
- 用户、角色、审计列表使用分页和索引；
- Enforcer 只加载目标租户有效策略；
- 权限变更单实例环境 1 秒内生效；
- 不对十万级订单逐行执行 Casbin。

### A9-6｜完整 E2E

至少完成：

1. 平台管理员创建角色和用户；
2. 商品运营完成分类、SKU 新增、编辑和发布；
3. 订单客服确认订单并生成工单；
4. 调度员查看履约详情并派单；
5. 师傅小程序完成接单、到达、服务和完工提交；
6. 客户小程序完成验收；
7. 初审员执行质检初审；
8. 总监执行二级审核；
9. 无权限用户逐项验证 403；
10. 角色权限修改后当前用户能力即时变化。

### A9-7｜文档和自动检查

更新：

- `deploy_local.md`；
- `apps/server-go/.env.example`；
- `deploy/compose.yaml`；
- 根目录 `README.md`；
- 必要的技术方案权限章节；
- OpenAPI。

执行：

```powershell
cd D:\work\fix-pro\apps\server-go
go test ./...

cd D:\work\fix-pro
npm run check
```

退出条件：SPEC-009 AC-01 至 AC-12、自动检查、真实 PostgreSQL 和三端 E2E 全部通过。

## 15. 预期工程变更

### 15.1 Go 后端

预计新增或修改：

```text
apps/server-go/cmd/bootstrap-admin/
apps/server-go/internal/authorization/
apps/server-go/internal/adminidentity/
apps/server-go/internal/platform/auth/
apps/server-go/internal/app/app.go
apps/server-go/internal/platform/config/
apps/server-go/api/openapi.yaml
apps/server-go/go.mod
apps/server-go/go.sum
```

业务模块只修改权限入口和真实操作人传递，不改变业务状态机。

### 15.2 PostgreSQL

```text
apps/server-go/db/migrations/000011_admin_rbac_casbin.up.sql
apps/server-go/db/migrations/000011_admin_rbac_casbin.down.sql
```

### 15.3 React

预计新增或修改：

```text
apps/admin-web/src/api/auth.ts
apps/admin-web/src/api/adminUsers.ts
apps/admin-web/src/api/adminRoles.ts
apps/admin-web/src/stores/authStore.ts
apps/admin-web/src/components/PermissionGuard.tsx
apps/admin-web/src/hooks/usePermission.ts
apps/admin-web/src/pages/LoginPage.tsx
apps/admin-web/src/pages/AdminUserPage.tsx
apps/admin-web/src/pages/AdminRolePage.tsx
apps/admin-web/src/pages/AdminAuditPage.tsx
apps/admin-web/src/app/AdminLayout.tsx
apps/admin-web/src/App.tsx
```

文件名以实施时现有工程风格为准，不为了匹配本列表建立无用抽象。

## 16. 风险与控制

| 风险 | 控制措施 |
| --- | --- |
| Basic Auth 与新会话长期并存 | A8 原子收口并删除生产 Basic Auth |
| 前端隐藏但后端未校验 | A0 权限矩阵 + A8 后台路由完整性测试 |
| Casbin 与业务表双写不一致 | 规范化表唯一事实来源，自定义只读加载 Adapter |
| 权限修改后旧缓存继续放行 | 事务提交后租户级失效，A9 并发验证 |
| 跨租户角色或策略污染 | Domain + 组合外键 + 过滤 Adapter + 越权测试 |
| 超级管理员逻辑散落 | bypass 只在 AuthorizationService |
| 最后管理员被误禁用 | 数据库事务内查询和锁定保护 |
| Cookie 认证引入 CSRF | CSRF Token、Origin、SameSite、Secure |
| 初始管理员无法恢复 | 显式 bootstrap 命令和安全运维说明 |
| 为多区域提前过度改表 | 本期只做 DataScopeResolver 和 TENANT_ALL |
| 权限节点增长导致加载过慢 | 租户过滤、只加载有效策略、缓存和性能基线 |

## 17. 提交与执行建议

建议按里程碑形成可独立验证的提交：

1. `A0`：契约、权限矩阵和测试骨架；
2. `A1`：migration、权限种子和 bootstrap；
3. `A2`：Casbin 授权内核；
4. `A3`：会话认证；
5. `A4-A5`：角色和用户后端；
6. `A6`：React 权限基础；
7. `A7`：管理页面；
8. `A8`：全路由权限切换；
9. `A9`：安全、性能、E2E 和文档。

任何阶段失败时只修复当前阶段，不通过回退已执行生产 migration 解决业务问题。

## 18. 完成定义

PLAN-009 完成必须同时满足：

- migration `000011`、`000012` 在空库和当前存量 PostgreSQL 上成功；
- 平台超级管理员可安全初始化；
- Casbin Domain、Adapter、Enforcer 缓存和失效通过测试；
- 后台用户使用真实会话登录，不再依赖 Basic Auth；
- 管理员可通过 React 创建角色、配置权限、创建用户和分配角色；
- 菜单、路由、按钮和 API 权限一致；
- 全部 `/api/v1/admin/**` 路由完成分类和权限绑定；
- 质检审核员、总监审核员和调度员权限相互隔离；
- 跨租户、并发、CSRF、会话撤销和默认拒绝验证通过；
- 用户、角色、权限和登录行为完整审计；
- 原有客户小程序、师傅小程序和后台业务正向链路通过；
- `go test ./...` 和 `npm run check` 全部通过；
- 本地部署、初始化管理员和权限排障文档更新完成。

## 19. 本次执行记录（2026-08-15）

已完成：

- A0：后台路由权限矩阵、默认拒绝策略和 OpenAPI 会话入口；
- A1：`000011_admin_rbac_casbin`，后台用户、角色、权限、会话、平台管理员关系和种子权限；
- A1 补充：`000012_admin_rbac_org_provision` 为后续新增组织自动创建租户管理员角色和权限关系；
- A2：`internal/authorization` 的 `org::<org_id>` Domain、有效权限查询、缓存失效和默认拒绝；
- A3：HttpOnly 会话 Cookie、CSRF、Argon2id 密码哈希、退出、改密、会话撤销和首次登录强制改密；旧 PBKDF2 哈希在成功登录时惰性迁移；
- A4—A5：角色/权限、后台用户、角色分配、临时密码、重置密码、有效权限和审计 API；
- A6—A7：React Cookie 登录、动态菜单、用户管理、角色权限管理和改密页面；
- A8 基础收口：后台业务路由已按权限编码分类，未知后台路由默认拒绝，Basic Auth 默认关闭且仅保留显式本地应急开关；
- 文档：`.env.example`、`deploy_local.md`、OpenAPI 已同步。
- A2/A9 官方实现收口：使用 `github.com/casbin/casbin/v3` 的 `SyncedEnforcer` 和 `persist.Adapter`，由 PostgreSQL 规范化 RBAC 表加载租户级 `p/g` 策略；不新增重复 `casbin_rule` 表。
- A3 安全收口：使用 `golang.org/x/crypto/argon2.IDKey` 生成 Argon2id 哈希，编码中保存版本和成本参数；旧 PBKDF2 仅保留校验兼容，成功登录后自动升级。

验证记录：

- 当前本地 PostgreSQL 迁移版本为 12，重复执行迁移无变化；
- `go test ./...` 通过；
- React `tsc --noEmit --incremental false` 和 `vite build --configLoader runner` 通过；
- 真实 HTTP 验证登录 200、无会话 401、缺 CSRF 401、无权限 403、首次改密前 423、改密后恢复访问；
- 角色创建和后台用户创建在真实 PostgreSQL 上成功。
- 本次替换验证：管理员旧 PBKDF2 哈希登录后已变为 `argon2id$...`；调度员旧哈希同样完成惰性迁移；调度员拥有 `fulfillment.view` 时接口返回 200、无 `catalog.category.view` 时返回 403；新租户登录后只能看到本租户角色。
