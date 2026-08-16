# SPEC-009｜后台用户、角色与 Casbin 多租户权限体系

**状态：** Draft for review  
**版本：** V1.0  
**日期：** 2026-08-15  
**适用工程：** `apps/server-go`、`apps/admin-web`、`apps/server-go/db/migrations`  
**关联文档：** `docs/技术方案-V1.md`、`docs/产品方案-V1.md`、`deploy_local.md`

---

## 1. 结论

本期使用 Casbin 建设 FixPro 管理后台的用户、角色与权限体系，替换当前“单一 Basic Auth 管理员 + 固定 ADMIN 角色 + 全量静态菜单”的临时实现。

目标链路为：

```text
超级管理员登录
→ 新增后台用户
→ 创建自定义角色
→ 为角色勾选菜单与操作权限
→ 为用户分配一个或多个角色
→ 用户重新登录或刷新权限
→ 后端使用 Casbin 校验每次业务操作
→ React 只展示用户有权访问的菜单和按钮
→ 权限变更和敏感操作全量留痕
```

核心技术决策：

1. Casbin 负责权限决策，不负责用户登录、密码、菜单元数据和审计日志。
2. `org_id` 是租户边界，并映射为 Casbin Domain；任何角色和策略必须属于一个明确租户。
3. PostgreSQL 中的规范化业务表是用户、角色、权限关系的唯一事实来源；通过自定义 Adapter 将有效策略加载到 Casbin，不同时维护一份含义重复的业务关系和 `casbin_rule` 关系。
4. 菜单权限、按钮权限和 API 权限使用同一套稳定权限编码；前端隐藏不是安全边界，后端必须再次鉴权。
5. 第一阶段通过角色给用户授权，不提供直接给单个用户添加 Allow/Deny 权限的例外机制。
6. 第一阶段实现租户级 Domain 隔离；多区域数据过滤保留扩展接口，但不在本期给全部订单、工单和师傅表强行补充 `region_id`。

## 2. 背景与当前问题

### 2.1 当前认证基线

当前管理后台认证存在以下临时实现：

- 管理员用户名和密码来自 `APP_ADMIN_USERNAME`、`APP_ADMIN_PASSWORD`；
- Go 后端使用 HTTP Basic Auth；
- 管理员主体没有真实用户 ID，角色固定为 `ADMIN`；
- React 将 Basic Auth 凭据编码后保存在 `sessionStorage`；
- 后端业务 Service 普遍使用 `p.Role != "ADMIN"` 判断权限；
- 管理后台菜单写死在 `AdminLayout.tsx`，登录后全部可见；
- 没有后台用户、角色、权限、会话和登录日志管理。

### 2.2 当前风险

1. 所有后台人员共享同一管理员账号，无法追溯真实操作人。
2. 调度员、审核员、总监和商品运营拥有完全相同的权限。
3. 前端没有菜单隔离，后端没有操作级权限隔离。
4. 管理员密码长期保存在环境变量和浏览器会话存储中。
5. 无法禁用单个后台人员，也无法单独撤销其会话。
6. 无法支持同一人员在不同租户下拥有不同角色。
7. 后续多区域、多企业组织接入时，现有固定 `ADMIN` 角色不可扩展。

## 3. 目标与非目标

### 3.1 业务目标

1. 超级管理员能够查看全部后台菜单并执行全部后台操作。
2. 超级管理员能够新增、编辑、启用、禁用和重置后台用户密码。
3. 有权限的管理员能够创建自定义角色，并为角色配置菜单和操作权限。
4. 一个用户可以拥有多个角色，有效权限取所有启用角色权限的并集。
5. 同一用户在不同租户中可以拥有不同角色，权限不得跨租户继承。
6. 用户只能看到有权访问的菜单，只能看到有权执行的操作按钮。
7. 即使绕过前端直接请求 API，后端也必须返回 403。
8. 用户、角色、权限和敏感登录行为必须可审计。

### 3.2 技术目标

1. Go 后端集成 `github.com/casbin/casbin/v3`。
2. 使用 Casbin RBAC with Domains 模型表达租户内的用户、角色与权限关系。
3. 使用 PostgreSQL 规范化表保存用户、角色、权限目录及关联关系。
4. 提供租户级 Enforcer 缓存和精确失效机制，权限变更后及时生效。
5. 用声明式权限中间件替换后台接口中的固定 `ADMIN` 判断。
6. React 使用 `/api/v1/admin/auth/me` 返回的菜单和权限集合渲染界面。
7. 保留未来接入区域数据权限、角色继承、企业 SSO 和多实例 Watcher 的能力。

### 3.3 非目标

本期不包含：

- 客户微信登录、师傅微信登录和 OpenID/UnionID 绑定；
- 企业客户门户账号和企业员工账号；
- Keycloak、LDAP、AD、SAML、OIDC 企业单点登录；
- MFA、短信验证码和硬件密钥；
- 用户自助注册和找回密码；
- 角色继承、角色嵌套和临时角色；
- 直接给单个用户配置额外 Allow/Deny 权限；
- 字段级脱敏权限；
- 将区域数据权限实际接入全部订单、工单、客户和师傅查询；
- 允许运营人员创建任意前端路由或任意权限编码；
- 修改客户、师傅和公开 API 的现有认证方式。

## 4. 核心概念与边界

### 4.1 认证与授权分离

| 能力 | 负责组件 |
| --- | --- |
| 用户名、密码、账号状态 | FixPro `admin_user` |
| 登录、退出、会话续期 | FixPro Go 后端 |
| 用户属于哪些租户 | FixPro PostgreSQL |
| 用户拥有哪些角色 | FixPro PostgreSQL + Casbin Adapter |
| 角色拥有哪些权限 | FixPro PostgreSQL + Casbin Adapter |
| 当前请求是否允许 | Casbin Enforcer |
| 菜单树和按钮权限 | Go 后端根据有效权限生成 |
| 权限变更审计 | FixPro `audit_log` / 专用安全审计记录 |

Casbin 不保存密码，不签发登录会话，也不直接生成 React 菜单。

### 4.2 租户、区域和数据范围

- 租户：使用现有 `organization.id`，在 Casbin 中表示为 `org::<org_id>`；
- 区域：未来租户内部的经营区域，例如华东区、杭州区域、萧山服务区；
- 功能权限：是否可以查看订单、派单、审核、管理用户，由 Casbin 判断；
- 数据范围：有权限后能看到哪些区域的数据，由数据范围解析器和 SQL 条件判断。

本期不得把“多区域”直接建模为 Casbin Domain。Domain 始终表示租户，区域属于租户内部的数据范围。否则一个用户跨区域查询时会产生大量重复策略，也无法高效过滤十万级订单列表。

### 4.3 菜单、操作和 API 权限

权限采用稳定的 `resource + action` 二元组：

```text
resource = work_order
action   = dispatch
编码展示 = work_order.dispatch
```

权限类型：

| 类型 | 示例 | 用途 |
| --- | --- | --- |
| `MENU` | `work_order.view` | 是否展示履约调度菜单、是否允许进入页面 |
| `ACTION` | `work_order.dispatch` | 是否展示派单按钮、是否允许调用派单接口 |
| `ACTION` | `work_order.qa_review` | 是否允许执行质检初审 |
| `ACTION` | `system.role.manage` | 是否允许修改角色权限 |

不单独维护一套脱离业务语义的 URL 权限。HTTP 路由在注册时显式绑定业务权限编码。

## 5. Casbin 模型

### 5.1 模型定义

建议模型文件：

```text
apps/server-go/internal/authorization/model.conf
```

模型语义：

```ini
[request_definition]
r = sub, dom, obj, act

[policy_definition]
p = sub, dom, obj, act, eft

[role_definition]
g = _, _, _

[policy_effect]
e = some(where (p.eft == allow)) && !some(where (p.eft == deny))

[matchers]
m = g(r.sub, p.sub, r.dom) && r.dom == p.dom && (p.obj == "*" || r.obj == p.obj) && (p.act == "*" || r.act == p.act)
```

命名约定：

```text
用户主体：user::<admin_user_id>
角色主体：role::<admin_role_id>
租户 Domain：org::<organization_id>
资源：order、work_order、catalog_sku、admin_user、admin_role 等
动作：view、create、update、confirm、dispatch、qa_review 等
```

示例策略：

```text
g, user::101, role::20, org::1

p, role::20, org::1, order, view, allow
p, role::20, org::1, order, confirm, allow
p, role::20, org::1, work_order, view, allow
p, role::20, org::1, work_order, dispatch, allow
```

### 5.2 策略来源

PostgreSQL 规范化表是唯一事实来源：

- `admin_user_role` 转换为 Casbin `g` 规则；
- `admin_role_permission` 联合 `admin_permission` 转换为 Casbin `p` 规则；
- 禁用用户、禁用角色、禁用权限不加载到有效策略；
- 自定义 Adapter 只加载有效策略到 Enforcer；
- 管理接口只修改规范化表，不同时维护第二份重复关系。

禁止出现以下双写：

```text
admin_role_permission 保存一份
casbin_rule 再保存一份相同关系
```

否则任意一次事务失败都会造成后台显示权限与实际 Casbin 权限不一致。

### 5.3 Enforcer 管理

实现 `EnforcerManager`：

- 按 `org_id` 管理租户级 Enforcer；
- 首次访问时使用过滤 Adapter 加载该租户策略；
- 用户或角色权限变更提交成功后，只失效对应租户 Enforcer；
- 下一次请求重新加载该租户策略；
- 并发重载使用 singleflight 或等价机制，避免同一租户重复加载；
- 加载失败时默认拒绝权限，不允许降级为全部放行；
- 不允许客户端通过 Header 任意指定自己不属于的租户。

第一期部署仍为单 Go 实例。多实例部署时增加 PostgreSQL Watcher、Redis Watcher 或消息总线通知，不在本期提前引入。

## 6. 用户、角色与权限模型

### 6.1 后台用户

新增独立 `admin_user`，不复用当前 `employee_account`：

- `employee_account` 当前专用于师傅，存在 WORKER 业务约束；
- 后台人员和师傅生命周期、登录入口、权限模型不同；
- 强行复用会增加角色约束和数据迁移风险。

后台用户字段：

| 字段 | 说明 |
| --- | --- |
| `id` | 主键 |
| `org_id` | 所属租户 |
| `username` | 租户内唯一登录名 |
| `display_name` | 姓名/展示名 |
| `mobile` | 可选手机号 |
| `email` | 可选邮箱 |
| `password_hash` | 密码哈希，不返回前端 |
| `status` | `ACTIVE`、`DISABLED`、`LOCKED` |
| `must_change_password` | 首次登录是否强制改密 |
| `failed_login_count` | 连续失败次数 |
| `locked_until` | 自动锁定截止时间 |
| `last_login_at` | 最近成功登录时间 |
| `version` | 乐观锁版本 |
| `created_by/updated_by` | 操作人 |
| `created_at/updated_at` | 创建和修改时间 |

约束：

- 用户名在租户内唯一；
- 新增用户必须分配至少一个启用角色；
- 禁用后立即禁止登录并使现有会话失效；
- 锁定用户到期后允许重新登录；
- 不物理删除后台用户；
- 已产生审计记录的用户信息必须可追溯。

### 6.2 角色

角色字段：

| 字段 | 说明 |
| --- | --- |
| `id` | 主键 |
| `org_id` | 所属租户 |
| `role_code` | 系统自动生成，租户内唯一 |
| `name` | 角色名称，租户内唯一 |
| `description` | 角色职责说明 |
| `type` | `BUILT_IN`、`CUSTOM` |
| `status` | `ACTIVE`、`DISABLED` |
| `version` | 乐观锁版本 |
| `created_by/updated_by` | 操作人 |
| `created_at/updated_at` | 创建和修改时间 |

规则：

- 自定义角色编码由系统生成，不要求运营人员填写；
- 角色名称需要表达职责，例如“商品运营”“履约调度”“质检审核员”“总监审核员”；
- 禁用角色后，该角色立即不再贡献有效权限；
- 有用户关联的角色可以禁用，但不能直接删除；
- 无用户、无审计依赖的自定义角色允许删除，删除前二次确认；
- 内置角色不可删除、不可修改关键编码；
- 第一阶段不支持角色继承。

### 6.3 权限目录

权限目录由代码清单和数据库迁移注册，不允许后台人员随意创建权限编码。

权限字段：

| 字段 | 说明 |
| --- | --- |
| `id` | 主键 |
| `permission_code` | 全局稳定编码，例如 `work_order.dispatch` |
| `resource` | Casbin obj，例如 `work_order` |
| `action` | Casbin act，例如 `dispatch` |
| `name` | 中文名称 |
| `type` | `MENU`、`ACTION` |
| `parent_id` | 权限树父节点 |
| `route_path` | MENU 对应 React 路由，可空 |
| `icon` | MENU 图标标识，可空 |
| `sort_order` | 菜单和权限排序 |
| `status` | `ACTIVE`、`DISABLED` |

规则：

- 权限编码不能由运营人员修改；
- ACTION 必须归属一个业务模块；
- 路由和菜单节点由前端真实页面决定，不能动态创建不存在的 React 页面；
- 一个 API 可以绑定一个权限，复杂操作可在 Service 层追加第二次业务校验；
- 权限下线前必须确认没有有效角色继续引用。

### 6.4 用户与角色

- 一个用户可分配多个角色；
- 一个角色可分配多个用户；
- 有效权限是所有启用角色的 Allow 权限并集；
- 用户详情展示“直接分配角色”和“最终有效权限”；
- 第一阶段不支持单用户例外权限；
- 修改用户角色必须使用事务、版本号和审计日志。

## 7. 内置角色

### 7.1 平台超级管理员

平台超级管理员用于系统初始化和紧急运维：

- 能进入所有租户；
- 能看到所有菜单并执行所有操作；
- 不通过普通角色管理页面创建；
- 只能由显式的安全初始化命令创建；
- 不能禁用或删除最后一个平台超级管理员；
- 所有跨租户操作必须记录目标租户；
- 前端仍通过 `/auth/me` 获取全量权限，不允许仅靠前端写死展示。

平台超级管理员可以在授权服务入口执行受控 bypass，但 bypass 必须位于统一 `AuthorizationService`，不得散落在业务代码中。

### 7.2 租户管理员

每个租户创建一个内置“租户管理员”角色：

```text
p, role::<tenant_admin_role_id>, org::<org_id>, *, *, allow
```

租户管理员：

- 只能管理当前租户；
- 拥有当前租户全部菜单和操作权限；
- 不能进入其他租户；
- 不能删除内置角色；
- 不能禁用最后一个有效租户管理员。

### 7.3 建议初始化角色

除租户管理员外，可提供权限模板，但模板不是不可修改的系统角色：

| 角色模板 | 建议权限 |
| --- | --- |
| 商品运营 | 分类、SKU 查看、新增、修改、发布和下架 |
| 订单客服 | 订单查看、确认，客户信息查看 |
| 履约调度 | 履约查看、派单、改派、查看师傅排班 |
| 质检审核员 | 履约查看、完工凭证查看、质检初审 |
| 总监审核员 | 履约查看、质检结果查看、总监审核 |
| 师傅管理员 | 师傅、工种、技能和证书管理 |

## 8. 首批权限目录

### 8.1 经营概览

| 权限编码 | 类型 | 中文名称 |
| --- | --- | --- |
| `dashboard.view` | MENU | 查看经营概览 |

### 8.2 服务商品

| 权限编码 | 类型 | 中文名称 |
| --- | --- | --- |
| `catalog_sku.view` | MENU | 查看维修 SKU |
| `catalog_sku.create` | ACTION | 新增维修 SKU |
| `catalog_sku.update` | ACTION | 修改维修 SKU |
| `catalog_sku.publish` | ACTION | 发布维修 SKU |
| `catalog_sku.off_shelf` | ACTION | 下架维修 SKU |
| `catalog_category.view` | MENU | 查看服务分类 |
| `catalog_category.create` | ACTION | 新增服务分类 |
| `catalog_category.update` | ACTION | 修改服务分类 |
| `catalog_category.status` | ACTION | 启停服务分类 |

### 8.3 订单与履约

| 权限编码 | 类型 | 中文名称 |
| --- | --- | --- |
| `order.view` | MENU | 查看订单中心 |
| `order.confirm` | ACTION | 确认订单并生成工单 |
| `work_order.view` | MENU | 查看履约调度 |
| `work_order.dispatch` | ACTION | 派单 |
| `work_order.reassign` | ACTION | 改派 |
| `work_order.reschedule` | ACTION | 调整预约时间 |
| `work_order.qa_review` | ACTION | 质检初审 |
| `work_order.director_review` | ACTION | 总监审核 |
| `work_order.customer_service_confirm` | ACTION | 客服确认二次上门 |

### 8.4 师傅与技能

| 权限编码 | 类型 | 中文名称 |
| --- | --- | --- |
| `worker.view` | MENU | 查看师傅管理 |
| `worker.create` | ACTION | 新增师傅 |
| `worker.update` | ACTION | 修改师傅 |
| `worker.status` | ACTION | 启用或禁用师傅 |
| `worker_skill.view` | MENU | 查看工种与技能 |
| `worker_skill.manage` | ACTION | 管理工种与技能 |

### 8.5 系统权限

| 权限编码 | 类型 | 中文名称 |
| --- | --- | --- |
| `admin_user.view` | MENU | 查看后台用户 |
| `admin_user.create` | ACTION | 新增后台用户 |
| `admin_user.update` | ACTION | 修改后台用户 |
| `admin_user.status` | ACTION | 启用、禁用或解锁后台用户 |
| `admin_user.reset_password` | ACTION | 重置后台用户密码 |
| `admin_user.assign_role` | ACTION | 分配用户角色 |
| `admin_role.view` | MENU | 查看角色权限 |
| `admin_role.create` | ACTION | 创建角色 |
| `admin_role.update` | ACTION | 修改角色 |
| `admin_role.assign_permission` | ACTION | 配置角色权限 |
| `admin_role.status` | ACTION | 启用或禁用角色 |
| `admin_role.delete` | ACTION | 删除未使用的自定义角色 |
| `admin_audit.view` | MENU | 查看权限审计日志 |

当前尚未实现的占位菜单可以注册 MENU 权限，但不能注册不存在的操作权限。

## 9. 认证与会话

### 9.1 登录方式

后台使用用户名和密码登录：

```http
POST /api/v1/admin/auth/login
```

请求必须包含目标租户标识。租户选择可使用租户编码，不允许客户端直接提交任意 `org_id` 后即获得该租户身份。

服务端校验：

1. 租户存在且启用；
2. 用户属于该租户；
3. 用户状态为 `ACTIVE`；
4. 密码正确；
5. 用户至少拥有一个启用角色，平台超级管理员除外。

### 9.2 密码安全

- 密码使用 Argon2id 或经安全评审确认的等价密码哈希算法；
- 每个密码使用独立随机盐；
- 数据库不保存明文密码和可逆密码；
- 新建用户使用一次性初始密码，并设置 `must_change_password=true`；
- 初始密码不能通过用户查询接口再次获取；
- 重置密码后撤销该用户全部现有会话；
- 连续登录失败达到阈值后临时锁定；
- 登录错误统一返回“用户名或密码错误”，不能泄露用户是否存在。

### 9.3 会话

管理后台使用服务端会话和不可预测的随机会话令牌：

- 浏览器使用 `HttpOnly` Cookie；
- 生产环境启用 `Secure`；
- `SameSite=Lax` 或更严格；
- Cookie 不保存角色和权限快照；
- 服务端会话保存用户 ID、租户 ID、过期时间和撤销状态；
- 权限每次从当前有效策略判断，角色调整后不需要等待旧 JWT 过期；
- 退出登录撤销当前会话；
- 禁用用户和重置密码撤销全部会话。

状态修改接口必须校验 CSRF Token 和请求 Origin。不能以 `SameSite` 作为唯一 CSRF 防护。

### 9.4 超级管理员初始化

新增显式初始化命令，例如：

```powershell
go run ./cmd/bootstrap-admin
```

规则：

- 只有系统尚无平台超级管理员时允许执行；
- 用户名和密码通过交互输入或安全环境变量提供；
- 不在数据库迁移 SQL 中写入固定生产密码；
- 初始化行为写安全日志；
- 完成切换后删除当前 Basic Auth 生产入口；
- `APP_ADMIN_USERNAME`、`APP_ADMIN_PASSWORD` 不再作为正常生产认证方式。

## 10. 后端授权执行

### 10.1 Principal

后台 Principal 至少包含：

```go
type Principal struct {
    OrgID           int64
    SubjectID       int64
    SubjectType     string
    Name            string
    IsPlatformAdmin bool
    SessionID       string
}
```

后台权限不得继续依赖单个字符串 `Role == "ADMIN"`。

### 10.2 权限中间件

路由注册示例：

```go
mux.Handle(
    "POST /api/v1/admin/work-orders/{id}/assign",
    adminSession(requirePermission("work_order", "dispatch", ful.Assign)),
)
```

执行顺序：

```text
恢复会话
→ 校验用户与租户状态
→ 构造 Principal
→ Casbin Enforce(sub, dom, obj, act)
→ 业务 Service 再校验资源归属和状态机
```

Casbin 只回答“当前用户在当前租户能否执行该类操作”，不能替代以下校验：

- 订单和工单是否属于当前 `org_id`；
- 客户是否属于当前租户；
- 工单是否处于可派单、可审核状态；
- 版本号、幂等键和并发控制是否合法。

### 10.3 默认拒绝

- 路由没有绑定权限时，后台保护路由在测试环境必须触发失败；
- Casbin 模型加载失败时返回 503 或明确内部错误，不允许默认放行；
- 无策略匹配返回 403；
- 跨租户请求返回 403 或 404，不能泄露资源存在性；
- 不能因前端未传权限信息而回退为超级管理员。

## 11. 区域数据权限扩展

本期定义接口，不改造全部业务聚合：

```go
type DataScopeResolver interface {
    ResolveRegions(ctx context.Context, principal Principal, resource string) (RegionScope, error)
}
```

未来区域范围建议：

| 范围 | 含义 |
| --- | --- |
| `TENANT_ALL` | 当前租户全部区域 |
| `SELECTED_REGIONS` | 角色配置的一个或多个区域 |
| `SELF` | 仅本人负责或本人创建的数据 |

数据列表查询应转换为 SQL 条件：

```sql
WHERE org_id = $1
  AND region_id = ANY($2)
```

禁止先查询全租户十万条订单，再对每一行调用 Casbin 判断区域权限。本期只保证 Casbin 策略和 Domain 设计不会阻碍后续区域能力；区域表、区域归属和各业务聚合的 `region_id` 由后续独立 Spec 定义。

## 12. React 管理后台

### 12.1 登录页

登录页支持：

- 租户编码；
- 用户名；
- 密码；
- 登录错误中文提示；
- 首次登录强制修改密码；
- 锁定和禁用状态提示；
- 登录成功后跳转到第一个有权访问的菜单。

移除 Basic Auth 凭据在 `sessionStorage` 中的保存逻辑。

### 12.2 当前用户接口

```http
GET /api/v1/admin/auth/me
```

返回示例：

```json
{
  "user": {
    "id": "101",
    "username": "dispatcher01",
    "displayName": "张调度"
  },
  "organization": {
    "id": "1",
    "name": "FixPro 总部"
  },
  "roles": [
    { "id": "20", "name": "履约调度" }
  ],
  "permissions": [
    "work_order.view",
    "work_order.dispatch",
    "worker.view"
  ],
  "menus": [
    {
      "key": "work-order",
      "label": "履约调度",
      "path": "/work-orders",
      "sortOrder": 40
    }
  ]
}
```

### 12.3 菜单与路由

- `AdminLayout` 使用服务端返回的菜单树；
- React 路由组件仍由代码注册，不从数据库加载任意组件；
- 用户直接访问无权路由时展示 403 页面；
- 用户没有任何可用菜单时展示“账号尚未分配可用权限”；
- 前端刷新页面后重新获取 `/auth/me`；
- 权限接口失败时不显示旧缓存中的全量菜单。

### 12.4 按钮权限

提供统一组件或 Hook，例如：

```tsx
const canDispatch = usePermission('work_order.dispatch')
```

要求：

- 无权限按钮默认不渲染；
- 不能在各页面重复手写角色名称判断；
- 菜单权限不自动代表所有操作权限；
- 即使按钮隐藏，后端 API 仍独立校验。

## 13. 管理页面

### 13.1 后台用户管理

页面能力：

- 用户列表、分页、用户名/姓名搜索；
- 状态和角色筛选；
- 新增用户；
- 编辑姓名、手机号、邮箱；
- 分配一个或多个角色；
- 查看最终有效权限；
- 启用、禁用、解锁；
- 重置密码；
- 查看最近登录时间和操作记录。

### 13.2 角色权限管理

页面能力：

- 角色列表、分页、状态筛选；
- 创建自定义角色；
- 编辑名称和说明；
- 使用树形控件配置菜单和操作权限；
- 展示角色用户数量；
- 查看角色最终权限；
- 启用、禁用和安全删除；
- 展示创建人、创建时间、修改人和修改时间。

权限树交互规则：

- 勾选操作权限时自动勾选所属菜单的查看权限；
- 取消菜单权限时提示将同时取消下属操作权限；
- 保存前展示新增和移除权限差异；
- 移除高风险权限时二次确认；
- 保存使用版本号，冲突返回 409 并要求刷新。

### 13.3 审计日志

至少记录：

- 登录成功、登录失败、退出和会话撤销；
- 新增、编辑、启用、禁用、解锁用户；
- 密码重置；
- 新增、编辑、启停、删除角色；
- 角色权限变更前后差异；
- 用户角色变更前后差异；
- 平台超级管理员跨租户操作。

记录字段至少包括：

```text
org_id、actor_user_id、actor_name、action、resource_type、resource_id、
before_json、after_json、request_id、ip、user_agent、created_at
```

密码、会话令牌和密码哈希不得进入审计内容。

## 14. PostgreSQL 数据模型

建议新增迁移：

```text
000011_admin_rbac_casbin.up.sql
000011_admin_rbac_casbin.down.sql
```

核心表：

| 表 | 用途 |
| --- | --- |
| `admin_user` | 后台用户 |
| `admin_user_session` | 服务端登录会话 |
| `admin_platform_super_admin` | 平台超级管理员关系或等价安全标记 |
| `admin_role` | 租户角色 |
| `admin_permission` | 全局权限目录 |
| `admin_user_role` | 租户内用户角色关系，转换为 Casbin g |
| `admin_role_permission` | 租户内角色权限关系，转换为 Casbin p |

关键约束：

- `admin_user(org_id, username)` 唯一；
- `admin_role(org_id, role_code)` 唯一；
- `admin_role(org_id, name)` 唯一；
- `admin_permission(permission_code)` 全局唯一；
- `admin_permission(resource, action)` 全局唯一；
- `admin_user_role(org_id, user_id, role_id)` 唯一；
- `admin_role_permission(org_id, role_id, permission_id)` 唯一；
- 所有关联表同时带 `org_id`，并使用组合外键确保不能跨租户关联；
- 关键表包含 `version`、创建人、修改人和时间字段；
- 会话令牌只保存哈希值。

`down.sql` 只用于空环境回滚。生产环境有后台用户和审计数据后，不允许直接执行破坏性降级。

## 15. API 契约

### 15.1 认证

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/api/v1/admin/auth/login` | 后台登录 |
| POST | `/api/v1/admin/auth/logout` | 退出当前会话 |
| GET | `/api/v1/admin/auth/me` | 当前用户、角色、权限和菜单 |
| PUT | `/api/v1/admin/auth/password` | 修改本人密码 |

### 15.2 用户

| 方法 | 路径 | 权限 |
| --- | --- | --- |
| GET | `/api/v1/admin/users` | `admin_user.view` |
| POST | `/api/v1/admin/users` | `admin_user.create` |
| GET | `/api/v1/admin/users/{id}` | `admin_user.view` |
| PUT | `/api/v1/admin/users/{id}` | `admin_user.update` |
| PUT | `/api/v1/admin/users/{id}/roles` | `admin_user.assign_role` |
| POST | `/api/v1/admin/users/{id}/status` | `admin_user.status` |
| POST | `/api/v1/admin/users/{id}/reset-password` | `admin_user.reset_password` |
| GET | `/api/v1/admin/users/{id}/effective-permissions` | `admin_user.view` |

### 15.3 角色和权限

| 方法 | 路径 | 权限 |
| --- | --- | --- |
| GET | `/api/v1/admin/roles` | `admin_role.view` |
| POST | `/api/v1/admin/roles` | `admin_role.create` |
| GET | `/api/v1/admin/roles/{id}` | `admin_role.view` |
| PUT | `/api/v1/admin/roles/{id}` | `admin_role.update` |
| PUT | `/api/v1/admin/roles/{id}/permissions` | `admin_role.assign_permission` |
| POST | `/api/v1/admin/roles/{id}/status` | `admin_role.status` |
| DELETE | `/api/v1/admin/roles/{id}` | `admin_role.delete` |
| GET | `/api/v1/admin/permissions/tree` | `admin_role.view` |
| GET | `/api/v1/admin/audit-logs` | `admin_audit.view` |

所有列表接口必须分页；所有修改接口必须携带版本号；所有高风险修改必须写审计日志。

## 16. 错误码

| 错误码 | HTTP | 含义 |
| --- | --- | --- |
| `ADMIN_AUTH_REQUIRED` | 401 | 未登录或会话失效 |
| `ADMIN_LOGIN_FAILED` | 401 | 用户名或密码错误 |
| `ADMIN_USER_DISABLED` | 403 | 用户已禁用 |
| `ADMIN_USER_LOCKED` | 423 | 用户暂时锁定 |
| `PASSWORD_CHANGE_REQUIRED` | 403 | 首次登录必须修改密码 |
| `PERMISSION_DENIED` | 403 | 缺少所需权限 |
| `TENANT_ACCESS_DENIED` | 403 | 无权进入目标租户 |
| `ADMIN_USER_NOT_FOUND` | 404 | 后台用户不存在 |
| `ADMIN_ROLE_NOT_FOUND` | 404 | 角色不存在 |
| `ADMIN_USERNAME_EXISTS` | 409 | 租户内用户名重复 |
| `ADMIN_ROLE_NAME_EXISTS` | 409 | 租户内角色名称重复 |
| `ADMIN_ROLE_IN_USE` | 409 | 角色仍被用户引用，不能删除 |
| `LAST_TENANT_ADMIN_PROTECTED` | 409 | 不能禁用最后一个租户管理员 |
| `LAST_PLATFORM_ADMIN_PROTECTED` | 409 | 不能移除最后一个平台超级管理员 |
| `RESOURCE_VERSION_CONFLICT` | 409 | 用户或角色已被并发修改 |
| `AUTHORIZATION_POLICY_LOAD_FAILED` | 503 | Casbin 策略加载失败 |

错误提示必须为中文友好文案，不向前端返回 SQL、Casbin 模型或密码校验内部信息。

## 17. 兼容与迁移策略

### 17.1 切换步骤

1. 新增数据库表和权限目录种子数据；
2. 接入 Casbin、Adapter、EnforcerManager 和会话认证；
3. 创建平台超级管理员；
4. 为全部现有后台 API 绑定权限；
5. 实现 React 登录、当前用户、动态菜单和按钮权限；
6. 实现用户、角色和审计页面；
7. 完成权限矩阵回归；
8. 删除生产 Basic Auth 和旧浏览器凭据存储；
9. 更新本地部署文档和环境变量清单。

### 17.2 兼容原则

- 不修改客户和师傅认证链路；
- 不改变订单、工单、SKU 和师傅业务 API 的请求体；
- 后台 API 路径保持不变，只将认证从 Basic Auth 切换为后台会话；
- 权限不足统一返回 403，不伪装成业务状态冲突；
- 前端遇到 401 清理本地用户状态并跳转登录；
- 前端遇到 403 保留登录状态并展示无权限提示；
- 不允许生产环境同时长期保留 Basic Auth 和新会话认证两套入口。

## 18. 并发、缓存与一致性

1. 角色权限保存、角色版本更新和审计日志必须在同一数据库事务内完成。
2. 用户角色保存、用户版本更新和审计日志必须在同一数据库事务内完成。
3. 事务提交成功后才能使租户 Enforcer 失效。
4. 如果失效通知失败，接口返回错误并记录告警，不允许长期保留旧策略。
5. 同一角色并发编辑使用 `version`，后提交者返回 409。
6. 禁用用户先提交状态和会话撤销，再拒绝其后续请求。
7. Enforcer 缓存不得跨 `org_id` 复用错误策略。
8. 权限变更生效目标为单实例环境不超过 1 秒。
9. 多实例一致性由后续 Watcher 实现，实施前不得直接水平扩容授权服务实例。

## 19. 安全要求

- 密码、初始密码、会话令牌不得写入普通日志；
- 登录和权限接口必须有速率限制；
- 登录成功后轮换会话标识，防止会话固定；
- 修改密码后撤销其他会话；
- Cookie 在生产环境必须使用 HTTPS 和 Secure；
- 所有管理查询强制带 `org_id`；
- 任何来自客户端的 org、角色和权限字段都需要服务端重新校验；
- 超级管理员 bypass 只能存在于统一授权模块；
- 禁止把 `isAdmin=true`、角色名或权限集合放在可篡改前端存储中作为后端依据；
- 权限目录变更必须通过代码评审和数据库迁移；
- 审计日志不可由普通管理员删除或修改；
- 媒体下载、导出和批量接口同样必须纳入权限校验。

## 20. 验收场景

### AC-01｜超级管理员全权限

```gherkin
Given 平台超级管理员已登录并进入租户 1
When 获取当前用户信息
Then 返回全部启用菜单和操作权限
And 可以创建用户、角色并修改角色权限
```

### AC-02｜自定义角色和用户授权

```gherkin
Given 超级管理员创建角色“履约调度”
And 只赋予 work_order.view、work_order.dispatch、worker.view
And 创建用户并分配该角色
When 该用户登录
Then 只看到履约调度和师傅查看相关菜单
And 可以查看工单和执行派单
And 看不到 SKU、用户和角色管理菜单
```

### AC-03｜前后端双重权限

```gherkin
Given 当前用户没有 work_order.qa_review
When 用户直接请求质检初审 API
Then 返回 HTTP 403 和 PERMISSION_DENIED
And 工单状态、版本和审核记录均不改变
```

### AC-04｜总监和审核员隔离

```gherkin
Given 质检审核员只有 work_order.qa_review
And 总监审核员只有 work_order.director_review
When 两个用户分别登录
Then 质检审核员不能执行总监审核
And 总监审核员不能执行质检初审
```

### AC-05｜角色权限即时生效

```gherkin
Given 用户正在使用履约调度页面
When 管理员从其角色中移除 work_order.dispatch
Then 该租户 Enforcer 被失效并重新加载
And 用户再次派单时立即返回 403
And 刷新当前用户信息后派单按钮消失
```

### AC-06｜用户禁用

```gherkin
Given 用户已有有效登录会话
When 超级管理员禁用该用户
Then 该用户全部会话被撤销
And 后续请求返回 401 或明确禁用状态
And 不能再次登录
```

### AC-07｜租户隔离

```gherkin
Given 用户 A 只属于租户 1
And 租户 1 和租户 2 都存在同名角色“履约调度”
When 用户 A 构造租户 2 的请求
Then 服务端拒绝访问
And 不加载或继承租户 2 的角色策略
```

### AC-08｜最后管理员保护

```gherkin
Given 租户只有一个有效租户管理员
When 尝试禁用该用户或移除其管理员角色
Then 返回 LAST_TENANT_ADMIN_PROTECTED
And 原用户和角色关系保持不变
```

### AC-09｜角色并发编辑

```gherkin
Given 两名管理员同时打开角色详情 version=3
When 第一名管理员保存成功
And 第二名管理员继续用 version=3 保存
Then 第二次请求返回 HTTP 409
And 不覆盖第一名管理员的权限变更
```

### AC-10｜审计完整

```gherkin
Given 管理员修改用户角色和角色权限
When 查询安全审计日志
Then 能看到操作人、租户、时间、请求 ID 和变更前后差异
And 日志中不存在密码、密码哈希和会话令牌
```

### AC-11｜策略加载失败默认拒绝

```gherkin
Given Casbin 策略加载失败
When 普通后台用户访问受保护接口
Then 接口不允许放行
And 返回明确错误并记录服务端告警
```

### AC-12｜回归验证

- 超级管理员仍可完成 SKU、分类、订单确认、派单、审核和师傅管理正向链路；
- 调度员只能完成已授权的履约操作；
- 审核员和总监权限相互隔离；
- 客户小程序和师傅小程序原有认证及履约链路不受影响；
- `go test ./...`、React lint、React build 和两个微信小程序 typecheck 全部通过。

## 21. 性能与容量

- Casbin 单次内存权限判断目标 P95 小于 5ms；
- `/api/v1/admin/auth/me` 在权限缓存命中时目标 P95 小于 200ms；
- 用户和角色列表必须分页，默认每页 20 条；
- 权限树允许至少 500 个权限节点；
- 单租户允许至少 10,000 个后台用户、500 个角色；
- Enforcer 只加载有效策略，不加载禁用角色和禁用用户关系；
- 未来租户数量增长时使用过滤 Adapter 和租户级缓存，不能每次启动无条件加载全部租户全部策略。

## 22. 测试要求

### 22.1 Go 单元测试

- Principal 与会话恢复；
- 密码校验和锁定；
- Casbin 模型 Allow、Deny、无策略默认拒绝；
- 同一用户在不同 Domain 下角色不同；
- 租户管理员通配策略；
- 普通角色菜单与操作权限；
- Enforcer 缓存命中和租户级失效；
- 禁用角色和禁用用户不产生有效策略；
- 最后管理员保护；
- 版本冲突。

### 22.2 PostgreSQL 集成测试

- 跨租户用户角色关联被组合外键拒绝；
- 角色权限变更事务回滚不产生半成品；
- 用户禁用和会话撤销一致；
- 同租户用户名、角色名和角色编码唯一；
- Adapter 加载的 p/g 规则与数据库关系完全一致；
- 并发修改角色只允许一个版本成功。

### 22.3 三端回归

- React 使用真实登录 Cookie；
- 菜单、路由、按钮和 API 权限一致；
- 401、403 和 409 中文提示正确；
- 客户小程序原链路不受影响；
- 师傅小程序原链路不受影响。

## 23. 完成定义

只有同时满足以下条件，本 Spec 才算完成：

1. 管理后台不再依赖共享 Basic Auth 管理员账号。
2. 超级管理员可以新增后台用户和自定义角色。
3. 角色可以配置菜单和操作权限，用户可以分配多个角色。
4. React 菜单和按钮依据服务端有效权限展示。
5. 全部后台业务 API 完成 Casbin 权限绑定，没有遗漏的默认放行路由。
6. 同一用户在不同租户下的角色策略严格隔离。
7. 禁用用户、修改角色和移除权限能够及时生效。
8. 用户、角色、权限和登录关键行为有完整审计。
9. 原有 SKU、订单、履约、审核和师傅管理链路通过超级管理员回归。
10. Go、React 和两个微信小程序的自动检查全部通过。
11. 本地部署文档已更新为新后台登录和超级管理员初始化方式。

## 24. 官方参考

- Casbin RBAC：https://www.casbin.org/docs/rbac
- Casbin RBAC with Domains：https://www.casbin.org/docs/rbac-with-domains
- Casbin Adapters：https://v3.casbin.org/docs/adapters
- Casbin Watchers：https://v3.casbin.org/docs/watchers
- NIST Role Based Access Control：https://csrc.nist.gov/projects/role-based-access-control
