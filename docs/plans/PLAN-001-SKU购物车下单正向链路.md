# PLAN-001｜维修 SKU 到购物车下单正向链路实施计划

**状态：** Ready for implementation  
**版本：** V1.1  
**日期：** 2026-08-03  
**对应 Spec：** `docs/specs/SPEC-001-SKU购物车下单正向链路.md` V1.3  
**适用工程：** `apps/server`、`apps/admin-web`、`apps/wechat-mini`

---

## 1. 交付目标

在本地/测试环境跑通并自动化验收以下真实纵向链路：

```text
React 后台创建“家庭基础漏水检测”SKU 并上传图片
→ 发布 SKU 版本
→ 微信小程序动态展示服务
→ 客户加入服务端购物车
→ 填写故障描述并上传图片/视频
→ 提交订单
→ React 后台查看订单、服务承诺快照和故障资料
```

本计划只实现 SPEC-001 范围，不提前实现微信正式登录、支付、派单、预约、企业合同价、搜索联想/同义词/热搜运营和完整权限系统。

V1.1 增量包括：后台分类 CRUD/排序/启停、SKU 分类动态选择、公共分类目录、基础名称搜索，以及参考 `docs/啄木鸟截图` 重构小程序“首页、全部服务、我的”三 Tab。购物车保留业务能力但从 TabBar 移除。

## 2. 当前基线

| 端 | 已有能力 | 本计划缺口 |
|---|---|---|
| Java 后端 | Spring Boot 3.5、Java 21、MyBatis、Flyway、Security、统一响应与错误处理 | Catalog 仍返回静态数据；无媒体、购物车、订单项和查询实现 |
| 数据库 | V1 已有组织、客户、分类、SKU、订单、幂等、审计和 Outbox 基表 | SKU 发布版本、媒体、购物车、订单项和业务种子数据缺失 |
| React 后台 | 登录、管理布局、请求封装、占位路由 | 无 SKU 管理和真实订单页面 |
| 微信小程序 | 首页、静态服务页、请求层、订单占位页 | 无详情、购物车、媒体上传、确认订单与结果页 |
| 基础设施 | MySQL、Redis、MinIO Compose | 本切片的本地媒体适配和测试目录尚未实现 |

## 3. 实施约束与默认决策

1. 后端保持 Java 模块化单体，不新增独立服务。
2. 管理后台使用 React；小程序使用微信原生 TypeScript。
3. 数据库金额一律使用整数分和 Java `long`；前端只负责格式化显示。
4. 数据库主键由 MySQL `AUTO_INCREMENT` 生成，API 中以字符串序列化，避免 JavaScript 大整数精度问题。V2 迁移同时调整本切片会写入的既有表主键策略。
5. SKU 可编辑字段作为工作副本；公共目录永远读取 `service_sku_version.snapshot_json` 指向的当前发布版本。编辑已发布 SKU 不立即影响小程序，再次发布后才生效。
6. 本地媒体使用 `ObjectStoragePort` 的文件系统适配器，根目录由配置指定且位于源码目录之外；测试使用临时目录。MinIO/S3 适配不阻塞本切片。
7. 后台继续使用现有 Basic Auth；小程序仅在 `local` Profile 使用 `Bearer local-customer-1`。
8. “家庭基础漏水检测 99 元/次”仅作为测试种子和演示数据。
9. 每一阶段合入前必须保持已有检查通过，不允许把三端集成问题全部留到最后处理。

## 4. 里程碑与关键路径

| 里程碑 | 结果 | 依赖 | 对应验收 |
|---|---|---|---|
| M0 数据与开发底座 | 空库迁移成功，本地管理员和客户身份可用 | 无 | 基础前置 |
| M1 SKU 管理闭环 | 后台可上传图片、创建并发布 SKU | M0 | AC-01 |
| M2 目录展示闭环 | 小程序展示后台发布的 SKU 和服务承诺 | M1 | AC-02 |
| M3 购物车与故障资料闭环 | 可加购、修改数量、填写描述、上传媒体 | M2 | AC-03、AC-08、AC-09 |
| M4 下单与后台查单闭环 | 服务端生成订单，后台看到完整快照 | M3 | AC-04～AC-07 |
| M5 可重复验收 | 自动化检查和人工演示脚本稳定通过 | M4 | AC-01～AC-09 |

关键路径为 `M0 → M1 → M2 → M3 → M4 → M5`。同一里程碑内，后端接口稳定后可并行开发 React 与小程序页面。

## 5. 工作分解

### P0｜工程护栏与契约确认

#### T0.1 固化 API 与领域枚举

交付内容：

- 建立 Catalog、Media、Cart、Order 的 DTO、状态枚举和错误码清单。
- 明确所有 ID 在 JSON 中为字符串，金额为整数分，时间为 ISO-8601 UTC。
- 将 Spec 错误码补入 `ErrorCode`，保持统一错误响应结构。
- 明确分页结构、空列表和字段命名，React 与小程序共用同一接口语义。

主要文件：

- `apps/server/src/main/java/com/fixpro/shared/web/ErrorCode.java`
- `apps/server/src/main/java/com/fixpro/catalog/**`
- `apps/server/src/main/java/com/fixpro/media/**`
- `apps/server/src/main/java/com/fixpro/cart/**`
- `apps/server/src/main/java/com/fixpro/order/**`

验收：后端编译通过；OpenAPI 能展示新增 DTO；不实现业务逻辑的接口不得返回伪成功数据。

#### T0.2 建立三端基线检查

执行并记录当前结果：

```powershell
npm run check
cd apps/server
./mvnw.cmd test
```

若当前环境缺少 JDK/MySQL，先记录环境阻塞；代码实现完成前必须补齐并执行，不以“未安装”为最终豁免。

### P1｜数据库与本地身份底座（M0）

#### T1.1 编写 Flyway V2 迁移

创建 `V2__sku_cart_order_slice.sql`，一次性完成：

- 扩展 `service_sku`：编码、简述、服务范围、除外项、售后/质保说明、单位、封面、当前发布版本和发布时间。
- 新增 `service_sku_version`、`service_sku_media`。
- 新增 `media_asset`。
- 新增 `shopping_cart`、`shopping_cart_item`、`shopping_cart_item_media`。
- 扩展 `customer_order` 联系人、手机号、服务地址和项目数。
- 新增 `order_item`、`order_item_media`。
- 为本切片实际写入的表配置可用的主键生成策略、唯一约束和查询索引。
- 插入组织 1 下的本地客户、首批分类；不直接插入已发布 SKU，SKU 必须通过后台链路创建。

注意事项：

- 不修改已执行的 `V1__baseline.sql`。
- 外键是否启用保持项目现有策略，但所有关联必须有索引和应用层归属校验。
- `snapshot_json` 保存完整发布快照；订单项关键承诺字段同时列式保存，方便后台查询与审计。

验收：

- 空 MySQL 数据库执行 V1、V2 成功。
- 重启服务不会重复插入种子或破坏数据。
- 唯一约束能够阻止重复 SKU 编码、重复购物车和重复版本。

#### T1.2 实现本地小程序身份

- 新增仅受 `local` Profile 控制的 `LocalMiniProgramAuthenticationFilter`。
- `Bearer local-customer-1` 映射到 `orgId=1/customerId=1`。
- 非 local Profile 不注册该 Filter。
- 封装当前管理主体与客户主体读取组件，Controller 不接受客户端传入的 `orgId/customerId`。

测试：local Token 成功、错误 Token 401、非 local 不接受本地 Token、客户资源不可越权。

### P2｜媒体能力（M1 前置）

#### T2.1 实现媒体领域和存储端口

- 定义 `ObjectStoragePort`：写入、读取、删除孤儿文件。
- 实现本地文件系统适配器与测试临时目录适配。
- 实现 `media_asset` Repository、用途、状态、所有者和元数据。
- 对上传文件校验大小、扩展名、声明 MIME 与文件签名；生成随机对象 Key。
- 禁止 SVG、HTML、可执行文件和双扩展名绕过。

#### T2.2 实现媒体 API

- `POST /api/v1/admin/media/images`
- `POST /api/v1/mini/media/fault`
- `DELETE /api/v1/admin/media/{id}`
- `DELETE /api/v1/mini/media/{id}`
- `GET /api/v1/public/media/{id}`
- `GET /api/v1/admin/media/{id}/content`
- `GET /api/v1/mini/media/{id}/content`

权限规则：

- 公共接口只读取当前已发布 SKU 引用的图片。
- 故障媒体只允许所属客户和有权限管理员读取。
- 上传失败不产生 READY 记录；孤儿 READY 文件超过 24 小时可被清理。

测试：文件签名、大小、用途、组织/客户归属、公共与私有读取、路径穿越、孤儿清理。

### P3｜Catalog 后端与 React SKU 管理（M1）

#### T3.0 实现服务分类管理

- 后端提供分类全部/启用列表、新增、编辑、排序和启停接口。
- 停用前检查已发布 SKU，存在引用时返回 `CATEGORY_IN_USE`。
- React 新增 `/catalog/categories`，展示状态、排序和 SKU 数，并支持维护。
- SKU 表单实时请求启用分类；小程序请求公共分类分组，三端不得硬编码分类枚举。

#### T3.1 实现 Catalog 持久化与发布

- 删除 `CatalogController` 中的 `List.of(...)` 静态服务。
- 实现分类查询、后台 SKU 分页、详情、新增、编辑、发布和下架接口。
- 创建默认状态为 `DRAFT`。
- 发布前校验固定价、服务承诺字段、封面、轮播数量与媒体归属。
- 发布事务中生成递增版本和完整 JSON 快照，更新当前发布指针，并写审计/Outbox。
- 公共目录只返回当前 `PUBLISHED` 版本快照。
- 调价或文案修改在再次发布前不得影响公共目录。

测试重点：

- 草稿不可见、发布可见、下架不可见。
- 重复编码、乐观锁、并发发布、发布字段缺失。
- 修改工作副本不污染当前发布版本。
- 服务范围、除外项、售后/质保说明完整进入快照。

#### T3.2 实现 React SKU 页面

新增路由：

- `/catalog/skus`
- `/catalog/skus/new`
- `/catalog/skus/:id/edit`

实现：

- SKU 分页列表、关键字筛选、状态标签和操作入口。
- 新建/编辑表单及 Spec 中的全部校验。
- Ant Design 图片上传、进度、失败重试、删除和轮播排序。
- 保存草稿、保存并发布、下架和未保存离开确认。
- 金额在元输入与分传输之间安全转换，禁止浮点误差。

验收：仅通过 UI 创建并发布“家庭基础漏水检测”；不允许使用 SQL 手工插入绕过后台链路。

### P4｜小程序动态目录（M2）

#### T4.0 重构三项导航与目录页面

- TabBar 只保留“首页、全部服务、我的”。
- 首页实现搜索、后台分类宫格、推荐服务、咨询兜底和低频购物车角标。
- 全部服务实现左分类、右服务网格；我的提供订单、购物车、售后与客服入口。
- 新增基础搜索页，按已发布 SKU 名称和简述搜索。
- 购物车从 TabBar 移除，保留服务详情和“我的”入口。

#### T4.1 改造请求层

- local 环境自动附带本地客户 Token；公共目录不强制认证。
- 统一解析 `ApiResponse` 和错误码。
- 增加上传封装，支持进度、失败和重试。
- API Base URL 继续按环境配置，不在页面硬编码。

#### T4.2 实现服务列表和详情

- 服务页调用 `GET /api/v1/catalog/services`，删除静态业务 SKU。
- 新增 `pages/services/detail` 并注册路由。
- 展示封面、轮播、名称、价格、单位、服务范围、除外项和售后/质保说明。
- 实现加载、空数据、失败重试和已下架状态。
- 实现数量 1～99 和加入购物车入口。

验收：后台发布后，小程序无需重新编译业务代码即可看到新 SKU；草稿和下架 SKU 不可见。

### P5｜购物车与故障资料（M3）

#### T5.1 实现 Cart 后端

- 实现购物车查询、加购、改数量、删除和故障资料保存接口。
- 首次加购锁定当前 SKU 发布版本和单价；重复加购累加数量。
- 所有小计和合计由服务端计算。
- 故障描述为 5～500 字符；每项至少关联一个 READY 图片或视频后才标记资料完整。
- 校验媒体用途、归属、状态、数量和跨客户访问。
- SKU 下架或版本变化时返回明确状态，不静默替换价格。

测试：重复加购、数量边界、金额计算、并发更新、越权、故障资料覆盖保存和媒体限制。

#### T5.2 实现小程序购物车

- 新增购物车页并增加可到达入口/角标。
- 展示 SKU 缩略图、版本价格、数量、小计、合计和资料完整状态。
- 支持数量加减、删除、故障描述、拍照/相册和视频选择。
- 使用 `wx.uploadFile` 展示进度、失败重试和删除。
- 资料不完整时禁止进入确认订单，并指明具体缺失项。

验收：刷新或重新进入页面后，购物车、故障描述和已上传媒体仍来自服务端。

### P6｜订单创建与后台订单中心（M4）

#### T6.1 实现 Order 创建事务

- `POST /api/v1/mini/orders` 强制 `Idempotency-Key`。
- 锁定购物车并重新校验资料、SKU 状态、版本和价格。
- 服务端重新计算金额。
- 同一事务写入订单、订单项、媒体关联、幂等结果并清空购物车。
- 订单项固化 SKU 编码、名称、版本、图片、价格、单位、服务范围、除外项、售后/质保说明和故障描述。
- 重复相同请求返回首次结果；相同 Key 不同请求返回 `ORDER_SUBMIT_DUPLICATED`。
- SKU 版本变化返回 `CART_SKU_CHANGED`，不创建订单且不清空资料。

测试：事务回滚、金额防篡改、空购物车、资料缺失、幂等、并发提交、调价冲突和历史快照不变。

#### T6.2 实现后台订单查询

- 实现订单分页列表，默认按创建时间倒序。
- 实现订单详情和受保护故障媒体读取。
- 列表手机号脱敏；详情在管理员权限下展示完整号码。
- 查询全部附带 `org_id` 范围。

#### T6.3 实现 React 订单页面

替换现有 `/orders` 占位页并新增 `/orders/:id`：

- 列表展示订单号、状态、联系人、脱敏手机号、金额和创建时间。
- 支持按订单号搜索并进入详情。
- 详情展示联系人、地址、完整订单项快照和故障描述。
- 故障图片使用受保护请求预览；视频支持受保护播放或下载。
- 不把永久私有媒体 URL 写入 DOM 或日志。

#### T6.4 实现小程序确认订单与结果页

- 确认页展示每项故障资料、金额、联系人、手机号和地址。
- 提交按钮防重复点击，每次业务提交生成 UUID 幂等键。
- 正确处理成功、字段错误、`CART_SKU_CHANGED` 和网络重试。
- 结果页展示订单号、状态和总金额。

### P7｜端到端验收与收尾（M5）

#### T7.1 自动化测试矩阵

后端：

- 领域单元测试：Catalog 发布、Cart 合计、Order 快照与幂等。
- Controller 权限与参数校验测试。
- MySQL 集成测试：Flyway、唯一约束、JSON、事务锁和调价冲突。
- 媒体安全测试：签名、大小、归属、公开/私有访问。

React：

- SKU 表单校验、金额转换、上传状态、发布流程。
- 订单列表/详情字段和媒体鉴权错误。
- 路由保护和 API 错误提示。

小程序：

- 请求与上传封装。
- 列表/详情状态。
- 购物车计算展示、故障资料完整性、下单防重和调价提示。

#### T7.2 执行完整检查

```powershell
npm run check
cd apps/server
./mvnw.cmd test
./mvnw.cmd package
```

使用真实 MySQL 测试 Flyway 和事务语义，不以 H2 代替最终集成验证。

#### T7.3 执行 AC 与人工演示

- 逐项执行 SPEC-001 的 AC-01～AC-09并保存结果。
- 严格按 Spec 第 18 节从后台 UI 新建 SKU 开始演示。
- 验证修改 SKU 后，已生成订单仍展示下单时的服务承诺和价格。
- 验证客户 B 无法读取客户 A 的故障媒体。

#### T7.4 文档与运行手册

- 更新根 README 的“当前已初始化/下一步”。
- 更新 `docs/runbooks/local-development.md`：媒体目录、测试 Token、三端启动顺序和演示账号。
- 记录新增配置项及 `.env.example`，不提交真实密钥或客户媒体。

## 6. 验收追踪矩阵

| Spec 验收 | 主要任务 | 自动化层级 | 通过证据 |
|---|---|---|---|
| AC-01 未发布不可见 | T3.1、T3.2 | Catalog 集成测试 | 草稿存在且公共目录结果不含该 SKU |
| AC-02 发布后可见 | T3.1、T3.2、T4.2 | API 集成 + 小程序页面测试 | 发布后返回图片、99 元测试价和三项服务承诺 |
| AC-03 正常加购 | T5.1、T5.2 | Cart 集成 + 页面测试 | 数量 1，小计/合计均为 9900 分 |
| AC-04 正常下单 | T6.1、T6.4 | Order MySQL 集成测试 | 唯一订单、9900 分、快照和媒体关联落库，购物车清空 |
| AC-05 后台可见 | T6.2、T6.3 | Admin API + React 测试 | 列表及详情展示订单和完整服务承诺快照 |
| AC-06 防重复订单 | T6.1 | 并发/幂等集成测试 | 相同 Key 只产生一张订单 |
| AC-07 调价保护 | T3.1、T5.1、T6.1 | Catalog/Order 联合集成测试 | 返回 `CART_SKU_CHANGED`，不建单且保留资料 |
| AC-08 故障资料必填 | T5.1、T5.2、T6.1 | 参数/领域/页面测试 | 缺描述或媒体均禁止下单 |
| AC-09 私有媒体访问 | T2.1、T2.2 | Security 集成测试 | 客户 B 为 403，管理员可读取 |
| AC-10 分类统一管理 | T3.0、T3.2、T4.0 | Catalog API + 三端页面测试 | 后台新增分类同时成为 SKU 选项和小程序目录 |
| AC-11 分类停用保护 | T3.0 | Catalog 集成测试 | 有已发布 SKU 时返回 `CATEGORY_IN_USE` |
| AC-12 三项底部导航 | T4.0 | 小程序配置/页面测试 | 仅首页、全部服务、我的，购物车仍可达 |

## 7. 推荐提交批次

为便于评审和回滚，按以下批次提交：

1. `feat(server): add sku cart order database slice and local mini identity`
2. `feat(server): add secured media upload and storage`
3. `feat(server): implement catalog draft publish and public queries`
4. `feat(admin): add sku management and image upload`
5. `feat(mini): render published services and details`
6. `feat(server): implement server-side cart and fault evidence`
7. `feat(mini): add cart and fault evidence workflow`
8. `feat(server): create idempotent orders and admin queries`
9. `feat(apps): add mini checkout and admin order center`
10. `test(docs): complete vertical-slice acceptance coverage`

每个批次只包含相关变更，不混入格式化或无关重构。

## 8. 风险与处理

| 风险 | 影响 | 处理方式 |
|---|---|---|
| SKU 工作副本与已发布内容混用 | 未发布修改提前影响客户 | 公共查询只读版本快照；发布事务切换指针 |
| 媒体直接暴露公共路径 | 泄露客户家庭故障资料 | 私有媒体鉴权读取；仅发布 SKU 图片可公开 |
| 前端传金额或浮点计算 | 订单金额不一致 | 服务端按整数分重算；前端仅展示 |
| 重复点击/网络重试 | 重复订单 | 幂等键、请求哈希和数据库唯一约束 |
| 调价后按旧价下单 | 成交争议 | 版本冲突返回 `CART_SKU_CHANGED`，保留购物车资料 |
| 本地 Token 误入生产 | 越权风险 | Profile 条件注册，并增加非 local 启动测试 |
| 视频过大导致联调不稳定 | 上传失败或内存压力 | 流式处理、50 MB 上限、不读取整文件进内存 |
| 99 元演示价被当作正式报价 | 经营与履约风险 | 标记为测试数据，生产由业务确认后后台配置 |

## 9. 完成判定

只有同时满足以下条件，计划才算完成：

- SPEC-001 AC-01～AC-09 全部通过。
- 管理后台可从 UI 创建、上传图片并发布 SKU。
- 小程序不包含硬编码业务 SKU、价格和服务承诺。
- 每个订单项都有故障描述、至少一个故障媒体及不可变 SKU/服务承诺快照。
- 金额由服务端计算，重复请求不会创建重复订单。
- 私有媒体跨客户访问被拒绝。
- 空库 Flyway、后端测试、React 构建/检查、小程序类型检查全部通过。
- README 和本地运行手册能让另一名开发者独立复现完整演示链路。

## 10. 明确不在本计划内

- 微信正式登录、手机号授权、生产级 Token。
- 微信支付、退款、优惠券、发票。
- 预约、工单拆分、派单、师傅履约。
- ToB 合同、项目、额度、SLA、批量下单。
- 搜索、套餐、区域价、起步价和检测后报价。
- 复杂家电维修、中央空调和厂房电路业务实现。
- MinIO/S3 生产适配、图片裁剪和视频转码。

任何新增需求先更新 Spec，再进入后续 Plan，不在实现阶段直接扩大本切片范围。
