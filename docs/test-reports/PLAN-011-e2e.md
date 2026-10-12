# PLAN-011 实现与本地验收记录

日期：2026-10-06。状态：代码实现及本地部署完成；真实微信授权、开发者工具和真机验收待完成。未发布小程序。

## 1. 交付范围

- 手机号快捷登录自动注册、组织内手机号唯一、跨微信环境复用账户；OpenID 不单独用于登录。
- AES-256-GCM 手机号存储、独立 HMAC 检索、随机会话 Token 哈希存储、期限/撤销/禁用/软删除校验。
- 客户私有路由统一认证，评价归属校验、头像私有下载及引用删除保护；购物车和后台订单查询改为当前组织。
- 游客浏览、主动授权、回跳、环境验证、会话代次及私有缓存清理；无客户退出、切换账号或改号入口/API。
- 点击“我的”头像编辑资料，微信 chooseAvatar/nickname 输入、草稿预览、保存/取消、版本冲突、失败重试及服务器最新资料刷新。
- 后台“用户管理”总体统计、筛选分页、资料与使用汇总、最近订单和私有头像；独立 `customer.view`，订单详情叠加 `order.view`。
- 迁移 000016、一次性初始化工具、媒体失败重试、macOS 启动脚本、配置及 OpenAPI/运维说明。

路由清单：[PLAN-011-routes.md](PLAN-011-routes.md)。手机号密钥和 AppSecret 不写入本报告。

## 2. 已完成检查

| 检查 | 结果及证据 |
| --- | --- |
| Go 全量单元及 PostgreSQL 集成测试 | `FIXPRO_INTEGRATION=1`、独立 `DB_DSN` 和 `CUSTOMER_TEST_DSN`，`go test -count=1 ./...` 通过 |
| 并发检测 | `go test -race ./internal/customer ./internal/platform/wechat ./internal/media` 通过 |
| 静态检查及编译 | `go vet ./...`；server/migrate/init-customer-auth 构建通过 |
| 小程序 | `npm run mini:typecheck` 通过 |
| 管理后台 | `npm run admin:lint`、`npm run admin:build` 通过 |
| 微信服务端配置 | 用已配置 AppID/AppSecret 调用 stable_token，凭据被接受；未输出 Token，未用此结果代替手机号授权验收 |
| 手机号资格及额度 | 用户确认已开通且有额度 |
| 本地服务 | API 健康检查 200；后台 `/customers` HTTP 200；旧 `local-customer-1` 私有请求 401 |
| 日志和前端密钥 | 请求日志只记录 Path，不记录搜索查询串；AppSecret 从小程序配置移至忽略提交的服务端文件 |

### 自动化场景与规格映射

| 范围 | 已覆盖 | 主要测试 |
| --- | --- | --- |
| SPEC 12.1 登录/会话 | 同号复用、同号 8 并发建档、不同手机号不同 ID、两个微信环境、环境不匹配、过期/撤销/禁用/软删除、跨组织拒绝、演示 Token 拒绝 | `customer/customer_test.go`、`customer/isolation_test.go` |
| 网关 | 两类 Code 分离、12 并发 Token 缓存、受控失效刷新、无效 Code、额度错误、AppID 不匹配、超时及受控错误、昵称正常检测 | `platform/wechat/client_test.go` |
| SPEC 12.1.1 资料 | 昵称长度/控制字符、加密随机 nonce/篡改、原子版本保护、他人/不存在媒体引用拒绝 | `customer/customer_test.go`、`customer/isolation_test.go` |
| 头像 | 实际 JPEG/PNG/WebP 解码、损坏/伪装、超过 5 MB、他人私有读取、引用中不能删除、24 小时未引用对象回收 | `media/avatar_test.go`、`customer/isolation_test.go` |
| SPEC 12.2 私有业务 | 地址删除、购物车修改、订单/工单/时间线/评价跨客户拒绝；正常评价成功、重复拒绝；全部 27 个已登记私有路由拒绝演示 Token | `customer/isolation_test.go`、`app/app_test.go`、`test/integration/f6_test.go` |
| SPEC 12.5 后台 | HMAC 精确搜索、分页聚合、订单实付/计数、客户 Token 不能当管理员、客户/订单权限组合、未赋权用户拒绝、新组织内置角色获得权限、跨组织详情/订单拒绝 | 上述客户集成测试及 F6 测试 |
| SPEC 12.3 初始化 | 原种子清理、手机号约束收口、重复执行保护、共享商品媒体保留、工单确认循环 FK、数据库提交后媒体失败及独立重试 | `customer/initialize_test.go::TestIntegrationCleanupMediaRetry` 在另一全新迁移库单独通过 |

普通集成测试库：`fix_pro_customer_auth_test`；一次性失败演练库：`fix_pro_customer_cleanup_test`。自动化不使用开发业务库做反复清空。清理测试要求单独的 `CUSTOMER_CLEANUP_TEST_DSN` 和全新迁移库；已完成初始化的库跳过破坏性演练。

统计采用北京时间自然日，注册含软删除/禁用，活跃排除软删除且包含窗口内有使用的禁用账户。活跃字段写入节流最多一分钟，不表示历史逐日 DAU。金额仅为订单已记录 `paid_amount`，不代表第三方核实流水。复杂日期边界、媒体引用竞争以及所有私有写入的完整逐路由组合仍可继续扩大回归矩阵；上表只声明实际已执行场景。

## 3. 本机一次性清理

已停止旧后端写入，迁移至 000016，预览后显式执行 `init-customer-auth -execute`，再启动新认证后端。

| 记录 | 清理前 | 清理后 |
| --- | ---: | ---: |
| 客户 | 1 | 0 |
| 客户地址 | 1 | 0 |
| 订单 / 工单 | 1 / 1 | 0 / 0 |
| 购物车 | 1 | 0 |
| 订单项 / 工单项 | 1 / 1 | 0 / 0 |
| 商品 / 后台管理员 / 师傅 | 1 / 1 / 0 | 1 / 1 / 0 |

客户会话及微信身份为 0；本次私有媒体对象清单为空。手机号必填 CHECK 已验证、两列 NOT NULL 已生效、一次性完成标记已保存。报告保存在忽略提交的 `.tmp/customer-auth-init-preview.json` 和 `.tmp/customer-auth-init-executed.json`。此后日常启动不会清空数据。

配置位于 `.tmp/customer-auth.env`，权限 600，密钥已独立生成并持久保存。`bash scripts/start-local.sh` 启动本地服务，API 为 `http://localhost:8080`，后台为 `http://localhost:5173/customers`。

## 4. 尚需真实微信验收

已具备 AppID、服务端 AppSecret，用户已确认手机号能力及额度。实际 `getPhoneNumber`、`wx.login`、隐私同意和头像选择需要真实微信环境及用户操作；本次未取得真实授权 Code，未消费手机号验证额度，也未声明真机通过。

1. 用微信开发者工具导入 `apps/wechat-mini`，验证手机号登录、只改昵称/头像/同时修改、保存、放弃、失败重试和再次进入资料。
2. 两个真实号码分别建立地址、购物车和订单，验证互不可见；同一号码在另一微信环境恢复同一客户及最新资料。
3. 验证断网、会话撤销/过期、延迟响应、上传途中失效、前后台切换以及原生返回确认。
4. 完成客户下单 → 后台派单 → 师傅完工 → 客户验收/评价三端正向回归。
5. 查看后台统计、私有头像、订单详情和实际管理员权限组合；必要时扩充日期边界/聚合回归。
6. 在微信后台配置正式隐私保护指引、完整用户协议和合法 HTTPS 请求/上传/下载域名；trial/release API 当前仍是占位域名，发布前替换。登录页服务说明不能替代正式完整用户协议。

内置浏览器尝试打开后台时 webview 附着超时，本次没有后台浏览器截图或开发者工具截图。构建成功与 HTTP 200 不等于 UI 人工验收。微信官方文档本次抓取不可用，平台原生能力及内容安全实际行为须以开发者工具/真机结果核实。

## 5. 发布状态

A0–A7B 的实现已落地；A8 自动化及本地检查已完成，人工微信验收待完成；A9 本地清理及文档已完成，正式域名/协议配置和发布验收待完成。未将整个 Plan 标记为全部验收或已发布。
