# SPEC-001｜维修 SKU 到购物车下单正向链路

**状态：** Draft for implementation  
**版本：** V1.3  
**日期：** 2026-08-03  
**适用工程：** `apps/server`、`apps/admin-web`、`apps/wechat-mini`  
**上游依据：** 产品方案 V1.2、技术方案 V1.2、设计方案 V1.2

---

## 1. 目标

先交付一条可在本地/测试环境完整演示、可自动化验收的正向链路：

```text
React 管理后台新增维修 SKU
→ 发布 SKU
→ 微信小程序服务列表和详情展示
→ 加入服务端购物车
→ 填写联系人与服务地址并提交订单
→ React 管理后台订单列表和详情看到该订单
```

本 Spec 的价值是验证三端与数据库的真实纵向集成，不追求一次完成全部商城、支付和履约功能。

## 2. 成功标准

必须同时满足：

1. 管理员可以在后台新增一个固定价维修 SKU 并发布。
2. 未发布 SKU 不出现在小程序；发布后无需重新编译小程序即可出现。
3. 小程序从后端加载 SKU，能查看名称、价格、计价单位和说明。
4. 用户可将 SKU 加入购物车、修改数量并看到服务端计算的小计和合计。
5. 用户提交联系人、手机号和服务地址后成功生成订单。
6. 后端订单保存 SKU 发布版本和成交快照，不依赖 SKU 后续变化。
7. 管理后台订单列表可看到新订单，详情可看到联系人、地址、订单项和金额。
8. 重复提交订单不会生成重复单据。
9. 管理员可以上传 SKU 封面图和轮播图，发布后小程序正常展示。
10. 客户必须针对每个维修订单项填写故障描述，并至少上传一张故障图片或一个故障视频才能下单。
11. 下单后故障描述和媒体与订单项永久关联，后台订单详情可以查看。
12. 已发布 SKU 必须明确服务范围、除外项和售后/质保说明；下单时将三者写入订单快照。
13. 首个演示 SKU 为“家庭基础漏水检测”，与合伙人当前已验证的测漏与水路售后能力一致。
14. 管理员可以新增、编辑、排序和启停服务分类；SKU 表单只能选择后台启用分类。
15. 小程序 TabBar 固定为“首页、全部服务、我的”，购物车不占 Tab，但从服务详情和“我的”保持可达。
16. “全部服务”按后台分类展示已发布 SKU，并支持按 SKU 名称/简述进行基础搜索。

## 3. 本期范围

### 3.1 包含

- 后台：分类列表、新增、编辑、排序、启停；SKU 列表、新增、编辑草稿、发布。
- SKU 字段：分类、编码、名称、简述、服务范围、除外项、售后/质保说明、计价模式、价格、单位、封面图、轮播图、状态。
- 小程序：首页、全部服务、基础搜索、我的、服务详情、购物车、确认订单、下单结果。
- 服务端购物车：添加、查询、修改数量、故障描述、故障图片/视频、删除、合计。
- 订单：创建、订单项与故障资料快照、后台列表、后台详情和媒体查看。
- 媒体：后台 SKU 图片上传、小程序故障图片/视频上传、对象存储适配和访问权限。
- 本地开发用小程序客户身份。
- Flyway 数据迁移、基础集成测试和三端联调验收。

### 3.2 暂不包含

- 微信正式登录、手机号授权和生产 Token。
- 微信支付、退款、优惠券、发票。
- SKU 审核流、定时发布、版本差异 UI、区域价格、套餐、搜索联想/同义词/热搜运营。
- 多地址、预约时段、订单拆工单、派单和师傅履约。
- SKU 图文详情编辑器、图片裁剪和视频转码；本期只支持封面/轮播图片以及故障图片/原始视频。
- 企业合同价、SLA 和批量工单。

以上能力不得通过当前接口“顺手做半套”；后续在本链路稳定后增量扩展。

### 3.3 首批业务边界

根据合伙人现有资源，首批 SKU 优先围绕以下已具备组织能力的方向建立：

1. 精准测漏与漏水诊断；
2. 水路维修、管件更换和局部改造；
3. 防水检查、维修和施工；
4. 暖气、净水清洗安装与维保。

家电清洗可以作为后续引流类目；复杂家电维修、中央空调、厂房电路和需要专项资质的项目，在确认人员、资质、设备、报价及质保标准前不进入本纵向切片。系统数据模型保持通用，但演示 SKU 不应暗示这些能力已经成熟。

## 4. 关键产品决策

### 4.1 SKU 单一事实来源

维修 SKU 只能由后端 `catalog` 模块写入。React 管理后台调用管理接口；微信小程序只调用公开目录接口。三端不得硬编码 SKU ID、名称或价格。

### 4.2 本期 SKU 生命周期

```text
DRAFT → PUBLISHED → OFF_SHELF
```

- 新建默认 `DRAFT`。
- `DRAFT` 可编辑，不对小程序可见。
- `PUBLISHED` 对小程序可见；每次发布生成不可变版本。
- `OFF_SHELF` 不允许新加购和新下单；历史订单仍可查询。
- 已发布 SKU 不物理删除。

本期后台允许管理员直接发布，不实现独立审核角色，但服务层和数据结构应保留未来扩展空间。

### 4.3 定价范围

本期完整验收只覆盖 `FIXED` 固定价 SKU。数据枚举保留：

- `FIXED`：固定价，可加购和下单。
- `STARTING_FROM`：本期可保存但禁止发布，返回 `SKU_PRICE_MODE_NOT_SUPPORTED`。
- `INSPECTION`：本期可保存但禁止发布，返回 `SKU_PRICE_MODE_NOT_SUPPORTED`。

这样避免在没有报价流程时错误计算起步价和检测后报价。

本文中的“家庭基础漏水检测 99 元/次”仅是本地/测试环境的链路验收数据，不构成生产定价。上线前必须由业务负责人确认服务边界、城市、成本和履约标准后配置真实价格。

### 4.4 购物车归属

购物车存储在服务端，以客户身份为唯一归属。小程序本地只缓存界面状态，不把本地缓存当作购物车事实来源。

### 4.5 下单状态

由于本期不接支付，订单创建后状态为 `WAITING_PAYMENT`，支付状态为 `UNPAID`。后台必须能正常查看，不因未支付而隐藏。

### 4.6 本地客户身份

当前工程只有后台 Basic Auth，微信 Bearer Token 尚未实现。为跑通本地链路：

- 仅在 Spring `local` Profile 注册 `LocalMiniProgramAuthenticationFilter`；
- 小程序使用 `Authorization: Bearer local-customer-1`；
- 服务端映射为 `customer_id=1`、`org_id=1`；
- 非 `local` Profile 不接受该 Token；
- 禁止通过 `X-Customer-Id` 或请求体传入客户 ID，避免形成越权接口。

### 4.7 媒体存储与可见性

所有图片和视频先上传形成 `media_asset`，业务表只保存媒体 ID，不保存 Base64、二进制或客户端本地路径。

- 定义 `ObjectStoragePort`，`local` Profile 使用工作区外可配置的本地文件目录，测试使用临时目录；生产适配 S3/COS/OSS 兼容对象存储。
- 本纵向切片使用服务端 `multipart/form-data` 上传，React 使用 Ant Design Upload，小程序使用 `wx.uploadFile`。
- SKU 图片是公开内容，只有 SKU 发布后才可通过公共媒体读取接口访问。
- 故障图片/视频是客户隐私资料，禁止公开 URL；仅订单所属客户和有权限的后台管理员通过鉴权接口获取短时访问结果。
- 媒体上传并校验成功后标记 `READY`；是否已被业务使用以关联表为准。后台任务清理超过 24 小时且不存在任何 SKU、购物车或订单关联的 READY 文件。
- 删除购物车项只解除临时故障媒体关联；已生成订单的媒体不得物理删除，只能受控作废并保留审计。

### 4.8 文件限制

| 用途 | 类型 | 单文件上限 | 数量 |
|---|---|---:|---:|
| SKU 封面/轮播 | JPEG、PNG、WebP | 10 MB | 封面 1 张，轮播最多 8 张 |
| 故障图片 | JPEG、PNG、WebP | 10 MB | 每个购物车项最多 6 张 |
| 故障视频 | MP4、MOV | 50 MB | 每个购物车项最多 2 个 |

每个购物车项图片和视频合计不超过 8 个。服务端必须校验声明 MIME、文件签名和大小；文件名不参与对象 Key 拼接。首期不强制视频时长，后续接入媒体元数据提取与转码后再限制。

### 4.9 下单资料完整性

每个选中的购物车项在下单前必须满足：

- `faultDescription` 去除首尾空格后为 5–500 字符；
- 至少有 1 个状态为 `READY` 且归属当前客户的故障图片或视频；
- 所有媒体上传均已完成，不存在 `UPLOADING / FAILED / SCANNING` 状态。

服务端是最终校验方，小程序按钮置灰不能代替服务端规则。

### 4.10 服务承诺版本化

服务范围、除外项和售后/质保说明是用户购买决策与售后履约依据，必须随 SKU 发布版本固化。下单时从当前发布版本复制到订单项快照；SKU 后续修改、调价或下架不得改变历史订单展示的承诺。

## 5. 用户故事

### US-01 管理员新增并发布 SKU

作为管理员，我希望新增“家庭基础漏水检测”固定价服务并发布，使小程序立即能够展示和购买。

### US-02 客户浏览服务

作为小程序客户，我希望看到后台已发布的维修服务及准确价格，从而选择需要的项目。

### US-03 客户加入购物车

作为客户，我希望把服务加入购物车并调整数量，系统准确计算金额。

### US-04 客户提交订单

作为客户，我希望填写服务联系人和地址后提交订单，并获得明确订单号。

### US-05 管理员查看订单

作为后台运营人员，我希望在订单列表和详情中看到客户刚提交的订单及商品快照。

### US-06 管理员维护 SKU 图片

作为商品运营人员，我希望上传 SKU 封面和轮播图，让小程序服务列表和详情具备基本的商品展示效果。

### US-07 客户提交故障资料

作为客户，我希望针对每个维修诉求上传现场图片或视频并填写故障描述，让后台和后续师傅准确理解问题。

### US-08 管理员维护服务分类

作为商品运营人员，我希望新增、编辑、排序和启停服务分类，使 SKU 表单和小程序目录使用同一套后台配置。

## 6. 页面规格

### 6.0 React 后台：服务分类 `/catalog/categories`

- 列表展示客户端分类名称、排序、状态和 SKU 数量。
- 支持新增、编辑名称/排序、启用和停用。
- 停用存在已发布 SKU 的分类时返回 `CATEGORY_IN_USE`，运营必须先下架或移动 SKU。
- SKU 新建/编辑页每次从后台读取启用分类，禁止在 React 代码中维护分类枚举。

### 6.1 React 后台：SKU 列表 `/catalog/skus`

列表字段：

| 字段 | 说明 |
|---|---|
| SKU 编码 | 唯一业务编码，如 `ELEC-SOCKET-REPLACE` |
| 名称 | 小程序展示名称 |
| 分类 | 当前只需支持选择已存在分类 |
| 计价 | 固定价及金额 |
| 状态 | 草稿、已发布、已下架 |
| 线上版本 | 未发布显示 `-` |
| 更新时间 | 后台更新时间 |
| 操作 | 编辑、发布、下架、查看 |

页面动作：

- “新增 SKU”进入新建页。
- 草稿可编辑和发布。
- 已发布可编辑形成新草稿；本期也可简化为重新发布新版本。
- 已发布可下架。
- 列表默认按更新时间倒序。

### 6.2 React 后台：SKU 编辑 `/catalog/skus/new`、`/catalog/skus/:id/edit`

字段：

| 字段 | 必填 | 规则 |
|---|---:|---|
| 分类 | 是 | 必须为启用分类 |
| SKU 编码 | 是 | 大写字母、数字、连字符，2–64 字符；创建后不可修改 |
| 名称 | 是 | 2–128 字符 |
| 简述 | 否 | 最多 500 字符 |
| 服务范围 | 是 | 10–1000 字符，明确本价格包含的服务 |
| 除外项 | 是 | 5–1000 字符，明确不包含的材料、拆改或复杂检测 |
| 售后/质保说明 | 是 | 5–500 字符，说明复核、维修或后续施工质保口径 |
| 计价模式 | 是 | 本期仅 `FIXED` 可发布 |
| 基础价格 | 是 | 人民币元输入，前端转整数分；1–99999999 分 |
| 计价单位 | 是 | 次、个、台、套，默认“次” |
| SKU 封面 | 是 | 上传 1 张图片；发布时必填 |
| 轮播图 | 否 | 最多 8 张，可拖拽排序；封面默认排第一 |

按钮：

- 保存草稿：保存成功后停留当前页面。
- 保存并发布：先保存，再调用发布接口；成功跳回 SKU 列表。
- 取消：有未保存内容时二次确认。
- 图片上传显示单文件进度、失败重试和删除；上传未完成时不能发布。

### 6.3 小程序：首页、全部服务与我的

- TabBar 固定为“首页、全部服务、我的”，不出现购物车和订单独立 Tab。
- 首页参考 `docs/啄木鸟截图/首页.jpg` 的结构：品牌区、大圆角搜索、分类宫格、推荐服务和咨询兜底，但使用 FixPro 视觉与真实能力，不复制促销/会员内容。
- “全部服务”参考 `全部服务.jpg`：顶部搜索，左侧后台启用分类，右侧该分类已发布 SKU 图片网格。
- “我的”参考 `我的.jpg`：账户摘要、订单入口、购物车、售后质保和客服入口；未实现功能必须明确提示，不能伪装可用。
- 购物车保留为页面内低频入口：服务详情加购后可进入，首页/全部服务显示轻量角标，“我的”提供入口。
- 基础搜索只按已发布 SKU 名称和简述匹配；不实现热搜运营、同义词和搜索联想。
- 所有页面实现加载、空数据和失败提示。

### 6.4 小程序：服务详情

- 展示封面/轮播图、名称、简述、价格、单位、服务范围、除外项和售后/质保说明。
- 服务范围、除外项和售后说明必须来自已发布 SKU，不能只写在图片或前端文案中。
- 数量选择默认 1，允许 1–99。
- “加入购物车”成功后显示提示，并刷新购物车角标。
- 若 SKU 已下架，显示“服务已下架”，禁止加购。

### 6.5 小程序：购物车

- 展示购物车项、SKU 缩略图、成交前当前价格、数量、小计、故障资料完整状态。
- 支持数量加减和删除。
- 每个购物车项提供“填写故障信息”，包含故障描述、拍照、从相册选择、拍摄/选择视频。
- 上传项显示本地缩略图、类型、进度、失败状态、重试和删除；离开页面后重新进入以服务端结果为准。
- 故障描述或媒体不完整时，购物车项显示明确提示，不能进入最终下单。
- 底部展示选中项数、合计和“去结算”。
- 本期默认全部选中，不实现多选和稍后购买。
- 购物车为空时提供“去选服务”。

### 6.6 小程序：确认订单

订单级字段：联系人、手机号、服务地址。确认页同时按订单项展示只读的故障描述和故障媒体缩略图。

- 联系人：2–64 字符。
- 手机号：本期校验中国大陆 11 位手机号格式。
- 服务地址：5–255 字符。
- 每个订单项必须有 5–500 字故障描述和至少一个已上传成功的故障图片或视频。
- 展示订单项和服务端返回的合计金额。
- 点击“提交订单”时生成新的 `Idempotency-Key`；按钮进入提交中并禁止重复点击。
- 成功跳转下单结果，展示订单号、金额、状态“待支付”。

### 6.7 React 后台：订单列表 `/orders`

字段：订单号、客户、联系人、手机号脱敏、地址摘要、订单项数、总金额、状态、下单时间、操作。

本期支持按订单号查询，默认按下单时间倒序。

### 6.8 React 后台：订单详情 `/orders/:id`

展示：订单号、状态、客户 ID、联系人、完整手机号（仅管理员）、服务地址、创建时间、订单项、SKU 编码、SKU 版本、名称快照、SKU 图片快照、服务范围快照、除外项快照、售后/质保说明快照、故障描述、故障图片/视频、单价、数量、小计和总金额。

故障图片可点击预览，视频可播放或下载；媒体请求必须携带后台认证，不在页面源码中暴露永久公共地址。

## 7. 数据模型

在现有 `V1__baseline.sql` 之后新增 Flyway 迁移，不修改已执行的 V1 文件。

### 7.1 `V2__sku_cart_order_slice.sql`

#### 调整 `service_sku`

新增：

```sql
sku_code VARCHAR(64) NOT NULL,
description VARCHAR(500) NULL,
service_scope VARCHAR(1000) NOT NULL,
exclusions VARCHAR(1000) NOT NULL,
warranty_description VARCHAR(500) NOT NULL,
unit VARCHAR(16) NOT NULL DEFAULT '次',
cover_media_id BIGINT UNSIGNED NULL,
current_published_version INT NULL,
published_at DATETIME(3) NULL
```

约束：`UNIQUE (org_id, sku_code)`。

状态使用：`DRAFT / PUBLISHED / OFF_SHELF`。

发布前 `cover_media_id` 必填，且媒体必须是当前组织上传的 `SKU_IMAGE + READY`。

#### 新建 `service_sku_media`

字段：`id、org_id、sku_id、media_id、media_role、sort_order、created_at`。

- `media_role`：`COVER / GALLERY`。
- 一个 SKU 只有一个 `COVER`，`GALLERY` 最多 8 个。
- `service_sku_version.snapshot_json` 同时保存发布时的封面和轮播媒体 ID 顺序。

#### 新建 `service_sku_version`

| 字段 | 类型 | 说明 |
|---|---|---|
| id | BIGINT UNSIGNED | 主键 |
| org_id | BIGINT UNSIGNED | 组织 |
| sku_id | BIGINT UNSIGNED | SKU |
| version_no | INT | 递增版本 |
| snapshot_json | JSON | 完整发布快照 |
| published_by | VARCHAR(64) | 当前 Basic Auth 用户名 |
| published_at | DATETIME(3) | 发布时间 |

唯一约束：`(org_id, sku_id, version_no)`。

#### 新建 `shopping_cart`

字段：`id、org_id、customer_id、version、created_at、updated_at`。

唯一约束：`(org_id, customer_id)`。

#### 新建 `shopping_cart_item`

字段：`id、org_id、cart_id、sku_id、sku_version、quantity、unit_price、fault_description、created_at、updated_at`。

唯一约束：`(org_id, cart_id, sku_id, sku_version)`。同一 SKU 再次加购增加数量，不插入重复行。

#### 新建 `shopping_cart_item_media`

字段：`id、org_id、cart_item_id、media_id、sort_order、created_at`。

唯一约束：`(org_id, cart_item_id, media_id)`。媒体必须属于当前客户并且用途为 `FAULT_EVIDENCE`。

#### 调整 `customer_order`

新增：

```sql
contact_name VARCHAR(64) NOT NULL,
contact_mobile VARCHAR(32) NOT NULL,
service_address VARCHAR(255) NOT NULL,
item_count INT NOT NULL DEFAULT 0
```

本地测试可直接存手机号；进入生产前必须按技术方案替换为密文和检索哈希。

#### 新建 `order_item`

字段：

```text
id, org_id, order_id, sku_id, sku_version,
sku_code_snapshot, sku_name_snapshot, unit_snapshot,
service_scope_snapshot, exclusions_snapshot, warranty_snapshot,
sku_cover_media_id_snapshot, fault_description,
unit_price, quantity, subtotal_amount, created_at
```

订单项所有金额为整数分，`subtotal_amount = unit_price × quantity`。

#### 新建 `order_item_media`

字段：`id、org_id、order_item_id、media_id、media_type、sort_order、created_at`。

下单事务把购物车项媒体关联复制为订单项媒体关联；媒体本身保持 `READY`，生命周期由业务关联与受控作废决定。

#### 新建 `media_asset`

| 字段 | 说明 |
|---|---|
| `id、org_id` | 主键与组织 |
| `owner_type、owner_id` | `ADMIN` 或 `CUSTOMER` 及主体 ID |
| `purpose` | `SKU_IMAGE / FAULT_EVIDENCE` |
| `media_type` | `IMAGE / VIDEO` |
| `object_key` | 对象存储 Key，不使用原文件名 |
| `original_filename` | 仅展示，输出时转义 |
| `mime_type、size_bytes、sha256` | 文件校验信息 |
| `status` | `READY / REJECTED / VOID` |
| `created_at、bound_at` | 生命周期时间 |

索引：`(org_id, owner_type, owner_id, status)`、`(org_id, purpose, created_at)`；`object_key` 唯一。

### 7.2 初始化数据

- 保留 `organization id=1`。
- 新增本地客户 `customer id=1`。
- 新增启用分类“精准测漏”；如果已有则迁移必须幂等或使用固定种子脚本。
- 不预置已发布 SKU，确保验收中的 SKU 确实来自后台创建。

## 8. API 规格

所有响应沿用现有 `ApiResponse<T>`。

### 8.1 媒体 API

#### `POST /api/v1/admin/media/images`

后台 Basic Auth，`multipart/form-data` 字段 `file`。仅接受 SKU 图片格式，成功返回：

```json
{
  "mediaId": "5001",
  "mediaType": "IMAGE",
  "mimeType": "image/jpeg",
  "sizeBytes": 245761,
  "previewUrl": "/api/v1/admin/media/5001/content",
  "status": "READY"
}
```

#### `POST /api/v1/mini/media/fault`

客户认证，`multipart/form-data` 字段 `file`。接受故障图片或视频，返回媒体 ID、类型、大小、状态和受保护预览地址。

#### `DELETE /api/v1/admin/media/{id}`、`DELETE /api/v1/mini/media/{id}`

只允许删除当前主体上传且未被 SKU、购物车或订单关联的媒体；已关联媒体只能解除允许解除的草稿关系或受控作废。

#### `GET /api/v1/public/media/{id}`

只提供已发布 SKU 绑定的图片。响应设置正确的 `Content-Type`、缓存头和 `X-Content-Type-Options: nosniff`。

#### `GET /api/v1/admin/media/{id}/content`、`GET /api/v1/mini/media/{id}/content`

受保护媒体读取。服务端校验组织、订单/购物车归属和角色后流式输出，不返回永久公开对象地址。

### 8.2 后台 SKU API

#### `GET /api/v1/admin/catalog/categories?includeDisabled=false`

默认返回启用分类选项；分类管理页传 `includeDisabled=true` 返回全部分类及 `sortOrder、status、skuCount`。

#### `POST /api/v1/admin/catalog/categories`

新增分类，字段为 `parentId、name、sortOrder`，默认 `ACTIVE`。

#### `PUT /api/v1/admin/catalog/categories/{id}`

编辑客户端分类名称、父级和排序。

#### `POST /api/v1/admin/catalog/categories/{id}/status`

请求 `{ "status": "ACTIVE|DISABLED" }`。存在已发布 SKU 时禁止停用。

#### `GET /api/v1/admin/catalog/skus?page=1&pageSize=20&keyword=`

返回分页 SKU 列表。

#### `POST /api/v1/admin/catalog/skus`

```json
{
  "categoryId": "1001",
  "skuCode": "LEAK-BASIC-DETECTION",
  "name": "家庭基础漏水检测",
  "description": "使用检测设备与现场排查定位常见家庭漏水问题",
  "serviceScope": "单一住宅地址、一个主要漏水问题的基础检测与初步结论记录",
  "exclusions": "不含开槽拆除、材料、维修施工及复杂暗管专项检测，超出范围另行报价",
  "warrantyDescription": "检测过程形成记录；后续维修或防水施工按对应服务项目另行约定质保",
  "priceMode": "FIXED",
  "basePrice": 9900,
  "unit": "次",
  "coverMediaId": "5001",
  "galleryMediaIds": ["5002", "5003"]
}
```

返回 SKU 详情，状态 `DRAFT`。

#### `PUT /api/v1/admin/catalog/skus/{id}`

仅编辑草稿或生成新的待发布内容；携带 `version` 做乐观锁。

#### `POST /api/v1/admin/catalog/skus/{id}/publish`

校验后生成版本，返回：

```json
{
  "skuId": "2001",
  "status": "PUBLISHED",
  "publishedVersion": 1,
  "publishedAt": "2026-08-02T03:00:00Z"
}
```

#### `POST /api/v1/admin/catalog/skus/{id}/off-shelf`

下架，不删除版本。

后台 API 继续使用现有 Basic Auth，并要求 `ROLE_ADMIN`。

发布校验要求服务范围、除外项和售后/质保说明均符合长度规则；封面媒体存在、已上传完成、属于当前组织且用途为 `SKU_IMAGE`。

### 8.3 小程序目录 API

#### `GET /api/v1/catalog/services?keyword=`

替换当前静态实现，返回：

```json
[
  {
    "id": "2001",
    "skuCode": "LEAK-BASIC-DETECTION",
    "name": "家庭基础漏水检测",
    "description": "使用检测设备与现场排查定位常见家庭漏水问题",
    "serviceScope": "单一住宅地址、一个主要漏水问题的基础检测与初步结论记录",
    "exclusions": "不含开槽拆除、材料、维修施工及复杂暗管专项检测，超出范围另行报价",
    "warrantyDescription": "检测过程形成记录；后续维修或防水施工按对应服务项目另行约定质保",
    "priceMode": "FIXED",
    "price": 9900,
    "unit": "次",
    "coverImageUrl": "/api/v1/public/media/5001",
    "galleryImageUrls": [
      "/api/v1/public/media/5001",
      "/api/v1/public/media/5002"
    ],
    "publishedVersion": 1
  }
]
```

`keyword` 为空返回全部已发布 SKU；非空时只按已发布版本的名称和简述进行包含匹配。

#### `GET /api/v1/catalog/categories`

按后台排序返回启用分类，每项包含 `id、name、services`；`services` 只包含归属该分类的当前已发布 SKU。

#### `GET /api/v1/catalog/services/{id}`

只允许读取当前已发布版本；下架返回 `SKU_NOT_AVAILABLE`。

### 8.4 购物车 API

均要求本地小程序客户身份。

#### `GET /api/v1/mini/cart`

返回购物车项、数量、小计和总计。

#### `POST /api/v1/mini/cart/items`

请求：

```json
{ "skuId": "2001", "quantity": 1 }
```

服务端读取当前发布版本和价格。重复加购累计数量，上限 99。

#### `PATCH /api/v1/mini/cart/items/{itemId}`

```json
{ "quantity": 2 }
```

#### `PUT /api/v1/mini/cart/items/{itemId}/fault`

```json
{
  "faultDescription": "厨房水槽下方持续渗水，关闭龙头后仍有水迹，已持续两天",
  "mediaIds": ["7001", "7002"]
}
```

服务端校验媒体属于当前客户、用途为 `FAULT_EVIDENCE`、状态为 `READY`、数量和类型符合限制。重复保存以请求中的媒体 ID 列表覆盖当前关联，但不得关联其他客户媒体。

#### `DELETE /api/v1/mini/cart/items/{itemId}`

删除当前客户自己的购物车项。

### 8.5 订单 API

#### `POST /api/v1/mini/orders`

Header：`Idempotency-Key: <UUID>`。

请求：

```json
{
  "contactName": "张女士",
  "contactMobile": "13800138000",
  "serviceAddress": "幸福小区 3 栋 2 单元 501"
}
```

创建逻辑：

1. 查询并锁定当前客户购物车。
2. 购物车为空返回错误。
3. 逐项校验故障描述和至少一个 READY 图片/视频完整。
4. 逐项校验 SKU 仍为 `PUBLISHED` 且版本、价格有效。
5. 重新计算每项小计和订单总计，不信任购物车或客户端传入合计。
6. 同一事务插入 `customer_order`、`order_item`、`order_item_media` 和幂等结果，订单媒体关联成为长期保留依据。
7. 清空已提交购物车项及其临时关联，但不得删除订单已绑定媒体。
8. 返回订单 ID、订单号、状态、总金额和创建时间。

#### `GET /api/v1/admin/orders`

后台分页查询订单，默认创建时间倒序。

#### `GET /api/v1/admin/orders/{id}`

返回订单、联系人、地址和完整订单项快照，其中包括成交名称与价格、服务范围、除外项、售后/质保说明、故障描述和故障媒体。

## 9. 错误码

| 错误码 | 场景 |
|---|---|
| `CATEGORY_NOT_FOUND` | 分类不存在或已停用 |
| `CATEGORY_IN_USE` | 分类存在已发布 SKU，不能停用 |
| `SKU_CODE_DUPLICATED` | SKU 编码重复 |
| `SKU_NOT_FOUND` | SKU 不存在 |
| `SKU_NOT_AVAILABLE` | SKU 未发布或已下架 |
| `SKU_PRICE_MODE_NOT_SUPPORTED` | 非固定价 SKU 尝试发布 |
| `SKU_VERSION_CONFLICT` | 并发编辑冲突 |
| `SKU_COVER_REQUIRED` | 发布 SKU 时未配置有效封面图 |
| `SKU_SERVICE_TERMS_REQUIRED` | 发布 SKU 时服务范围、除外项或售后/质保说明缺失或长度不合法 |
| `CART_EMPTY` | 空购物车提交订单 |
| `CART_ITEM_NOT_FOUND` | 购物车项不存在或不属于当前客户 |
| `CART_SKU_CHANGED` | SKU 版本或价格变化，需要刷新购物车 |
| `FAULT_DESCRIPTION_REQUIRED` | 订单项缺少故障描述或长度不合法 |
| `FAULT_MEDIA_REQUIRED` | 订单项未上传任何故障图片或视频 |
| `MEDIA_NOT_FOUND` | 媒体不存在 |
| `MEDIA_ACCESS_DENIED` | 媒体不属于当前主体或无订单访问权 |
| `MEDIA_TYPE_NOT_SUPPORTED` | 文件类型不在白名单 |
| `MEDIA_SIZE_EXCEEDED` | 文件超过用途对应上限 |
| `MEDIA_COUNT_EXCEEDED` | SKU 或故障媒体数量超过限制 |
| `MEDIA_NOT_READY` | 文件仍在上传、校验失败或已作废 |
| `ORDER_SUBMIT_DUPLICATED` | 幂等键已使用且请求不一致 |
| `INVALID_CONTACT_MOBILE` | 手机号格式错误 |

## 10. 后端模块与代码边界

建议新增：

```text
com.fixpro.catalog
├─ api/admin
├─ api/public
├─ application
├─ domain
└─ infrastructure

com.fixpro.cart
├─ api
├─ application
├─ domain
└─ infrastructure

com.fixpro.order
├─ api/admin
├─ api/mini
├─ application
├─ domain
└─ infrastructure

com.fixpro.media
├─ api/admin
├─ api/mini
├─ application
├─ domain
└─ infrastructure
```

规则：

- Controller 不直接操作 Mapper。
- 订单 Application Service 可以只读调用 Catalog Facade 获取当前发布快照。
- Cart 和 Order 不直接更新 `service_sku`。
- Catalog、Cart 和 Order 只保存 `mediaId`；文件读写通过 Media Facade 与 `ObjectStoragePort`。
- `CatalogController` 不再包含静态测试数据。
- DTO 与数据库 DO 分离；金额使用 `long`。

## 11. 事务与并发

- 发布 SKU：更新当前发布指针、插入版本和审计/Outbox 在同一事务。
- SKU 发布事务校验封面/轮播媒体并将媒体绑定到发布版本；文件本身已在上传阶段写入对象存储。
- 加购物车：以当前发布版本为准，使用唯一约束处理并发重复加购。
- 下单：订单、订单项、购物车清理和幂等记录在一个事务。
- 下单事务复制故障描述与媒体关联；事务失败时保持购物车及其媒体关联原状。
- 订单号在服务端生成并有唯一约束，建议格式 `FPyyyyMMddxxxxxxxx`。
- SKU 调价发生在加购后、下单前时，本期返回 `CART_SKU_CHANGED`，由小程序刷新购物车并提示用户重新确认，不静默使用新价格。

## 12. 安全要求

- 后台 SKU 写接口和订单查询接口必须认证。
- 小程序目录 GET 可匿名访问；购物车和下单必须识别客户。
- 所有资源查询带 `org_id` 数据范围。
- 服务端忽略客户端提交的 SKU 名称、价格、订单总额和客户 ID。
- SKU 公共媒体只允许已发布版本引用的图片；故障媒体必须鉴权并校验订单/购物车归属。
- 上传内容使用随机对象 Key，校验文件签名与 MIME，禁止 SVG、HTML、可执行文件和双扩展名绕过。
- 下载响应禁止 MIME 嗅探，原始文件名经过响应头安全编码；日志不记录对象存储签名 URL。
- 后台订单列表手机号默认脱敏，详情仅管理员可查看完整号码。
- 本地 Token 只能在 `local` Profile 生效，生产启动时不得注册对应 Filter。

## 13. 埋点与日志

最少记录：

- `admin_sku_created`
- `admin_sku_published`
- `admin_sku_media_uploaded`
- `mini_service_viewed`
- `mini_cart_item_added`
- `mini_fault_media_uploaded`
- `mini_fault_info_completed`
- `mini_checkout_started`
- `mini_order_submitted`

业务日志必须带 `requestId、orgId、principalId/customerId、skuId/orderId`，不得打印完整手机号。

## 14. 验收场景

### AC-01 未发布不可见

```gherkin
Given 管理后台已保存一个状态为 DRAFT 的“家庭基础漏水检测”SKU
When 小程序请求服务列表
Then 列表中不存在该 SKU
```

### AC-02 发布后可见

```gherkin
Given 管理员为“家庭基础漏水检测”填写服务范围、除外项和售后/质保说明，上传封面图并以 99 元/次发布
When 小程序重新加载服务列表
Then 能看到后台上传的封面图、“家庭基础漏水检测”和“¥99.00/次”
And 详情页能看到该版本的服务范围、除外项和售后/质保说明
```

### AC-03 正常加购

```gherkin
Given “家庭基础漏水检测”处于 PUBLISHED
When 客户加入 1 次到购物车
Then 购物车只有一个项目
And 数量为 1
And 小计和合计均为 9900 分
```

### AC-04 正常下单

```gherkin
Given 客户购物车有 1 次“家庭基础漏水检测”
And 客户填写了有效故障描述
And 客户上传了至少一张故障图片或一个故障视频
When 客户提交合法联系人、手机号和地址
Then 系统生成唯一订单号
And 订单状态为 WAITING_PAYMENT
And 总金额为 9900 分
And 订单项保存 SKU 编码、名称、版本、单价、数量、服务范围、除外项和售后/质保说明快照
And 订单项保存故障描述和故障媒体关联
And 购物车被清空
```

### AC-05 后台可见

```gherkin
Given 客户已经成功提交订单
When 管理员打开订单列表和订单详情
Then 能看到该订单号、联系人、地址、状态、订单项和 99 元总金额
And 能看到下单时的服务范围、除外项和售后/质保说明
```

### AC-06 防重复订单

```gherkin
Given 第一次下单请求已成功
When 客户使用相同 Idempotency-Key 和相同请求再次提交
Then 返回第一次的订单结果
And 数据库只有一张订单
```

### AC-07 调价保护

```gherkin
Given 客户以 99 元版本加入购物车
And 管理员把 SKU 调整为 129 元并发布新版本
When 客户提交订单
Then 返回 CART_SKU_CHANGED
And 不创建订单
And 小程序提示价格已变化并刷新购物车
```

### AC-08 故障资料必填

```gherkin
Given 客户购物车有“家庭基础漏水检测”
And 该购物车项没有故障描述或没有故障图片/视频
When 客户提交订单
Then 返回 FAULT_DESCRIPTION_REQUIRED 或 FAULT_MEDIA_REQUIRED
And 不创建订单
And 购物车资料保留
```

### AC-09 私有媒体访问控制

```gherkin
Given 客户 A 的订单包含故障视频
When 客户 B 请求该视频
Then 返回 403 和 MEDIA_ACCESS_DENIED
When 有权限的管理员在订单详情请求该视频
Then 可以正常查看
```

### AC-10 分类统一管理

```gherkin
Given 管理员在分类管理新增并启用“精准测漏”
When 管理员打开新增 SKU 页面
Then 分类下拉中能看到“精准测漏”
When 管理员将该 SKU 发布
Then 小程序“全部服务”左侧分类中能看到“精准测漏”及其服务
```

### AC-11 分类停用保护

```gherkin
Given “精准测漏”分类下存在已发布 SKU
When 管理员尝试停用该分类
Then 返回 CATEGORY_IN_USE
And 分类和已发布服务仍保持可见
```

### AC-12 三项底部导航

```gherkin
When 客户打开小程序
Then TabBar 依次只显示“首页、全部服务、我的”
And TabBar 不显示购物车
And 客户仍可从服务详情或“我的”进入购物车
```

## 15. 自动化测试要求

### 后端

- Catalog 发布领域测试。
- SKU 服务范围/除外项/售后质保说明必填、封面必填、图片数量/类型/大小和发布媒体绑定测试。
- Admin SKU Controller 权限与校验测试。
- Public Catalog 只返回已发布 SKU 的集成测试。
- Cart 重复加购、数量边界、越权测试。
- Media 文件签名、大小、所有权、公共/私有访问和孤儿清理测试。
- Cart 故障描述、媒体关联、数量限制和跨客户媒体越权测试。
- Order 创建事务、SKU 服务承诺与故障资料快照、媒体绑定、清空购物车、幂等和调价冲突集成测试；SKU 后续修改不得影响历史快照。
- Admin Order 列表和详情测试。

建议使用 Testcontainers PostgreSQL；若首期暂未引入，至少使用真实 PostgreSQL 测试 Profile 验证 migration 与 JSONB/锁语义，不用内存数据库代替 PostgreSQL 行为。

### React 后台

- SKU 表单金额元/分转换测试。
- SKU 图片上传进度、失败重试、封面校验和轮播排序测试。
- 保存草稿、发布成功与错误提示测试。
- 订单列表和详情渲染测试。

### 微信小程序

- 目录加载、空态和错误态。
- 加购物车、数量修改和合计展示。
- 故障描述、图片/视频上传、失败重试和资料完整性提示。
- 下单按钮防重复、故障资料必填、成功结果和 `CART_SKU_CHANGED` 提示。

## 16. 实施顺序

1. Flyway V2：SKU 版本、媒体、购物车、故障资料、订单项和订单联系人快照。
2. 本地客户身份与种子客户。
3. Media 领域、对象存储端口、本地适配、后台图片上传和小程序故障媒体上传。
4. Catalog 持久化、后台 SKU API、图片关联、发布 API；删除静态目录。
5. React SKU 列表、图片上传与编辑发布页面。
6. 小程序服务列表、轮播图与详情。
7. Cart 领域、故障描述/媒体 API 与小程序购物车。
8. Order 创建、故障资料快照、幂等、后台查询 API。
9. 小程序确认订单与结果页。
10. React 订单列表、详情和故障媒体查看。
11. 自动化测试与三端 E2E 验收。

## 17. Definition of Done

- 所有 AC-01 至 AC-12 通过。
- 数据库从空库执行全部 Flyway 成功。
- 后端测试、React 检查、小程序 TypeScript 检查全部通过。
- `CatalogController` 无静态 SKU。
- 小程序无硬编码业务 SKU 和价格。
- SKU 封面/轮播来自后台上传并随发布版本展示。
- SKU 发布版本及订单项均包含服务范围、除外项和售后/质保说明，历史订单不受 SKU 后续变化影响。
- 首个演示 SKU 和首批类目边界与合伙人已验证的测漏、水路、防水及暖气/净水维保能力一致。
- 每个订单项都保存有效故障描述及至少一张图片或一个视频。
- 故障媒体无永久公开地址，跨客户访问测试通过。
- 订单金额全部由服务端计算并保存版本快照。
- 后台新增 SKU 后，无需修改或重新编译小程序业务代码即可展示。
- README/本地运行手册补充三端演示步骤和测试账号。

## 18. 演示脚本

1. 启动 PostgreSQL、Go 后端、React 后台和微信开发者工具。
2. 使用 `admin/change-me-in-production` 登录本地后台。
3. 上传真实或合规的漏水检测作业图片作为封面和轮播图，新建固定价 SKU“家庭基础漏水检测”，填写服务范围、除外项和售后/质保说明，定价 99 元/次，保存并发布。
4. 打开小程序服务页，刷新后看到该 SKU 及后台上传的封面图。
5. 进入详情查看轮播图、服务范围、除外项和售后/质保说明，数量设为 1，加入购物车。
6. 在购物车填写具体漏水现象、发生位置和持续时间，拍摄/选择一张现场故障图片并上传；可额外上传一个故障视频。
7. 确认 99 元，填写联系人、手机号和地址，提交订单。
8. 记录返回订单号。
9. 回到管理后台订单中心，搜索订单号并打开详情。
10. 核对订单状态、联系人、地址、SKU 版本、服务范围/除外项/售后质保快照、数量、99 元总金额、故障描述和故障图片/视频。

完成以上步骤即视为本纵向切片跑通。
