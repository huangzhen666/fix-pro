# 微信支付与本地模拟运行手册

本功能使用 Go 保存支付事实，CloudBase HTTP 函数连接微信支付。本地模拟不会扣款，真实支付没有自动降级为模拟的分支。

## 本地启动

从项目根目录运行：

```bash
bash scripts/start-local.sh --migrate
```

若网络无法下载依赖而系统模块缓存已完整，可复用缓存启动：

```bash
GOMODCACHE="$(go env GOMODCACHE)" bash scripts/start-local.sh --migrate
```

脚本现在明确绑定 `127.0.0.1:8080`，默认显式注入 `PAYMENT_MOCK_ENABLED=true`；直接运行 Go 的默认配置仍关闭模拟支付。脚本不会替换已经运行的 API 进程：升级代码后应先停止由 `.tmp/server-local.pid` 标识的本项目 API，再运行脚本。不要停止不属于本项目的进程。

迁移 `000022` 对旧订单设置 `payment_required=false`，保留原有付款事实；迁移后的新订单默认需要付款，并在创建时写入 15 分钟支付有效期。迁移前备份数据库，先迁移再启动新代码。已有支付事实后，回滚迁移会被主动拒绝。

用微信开发者工具导入 `apps/wechat-mini`，完成现有手机号登录，选择服务并下单。下单后进入订单详情，点击“立即支付”，选择以下模拟结果：

- 模拟支付成功：实际写入 MOCK 支付单及流水，订单可继续后台确认。
- 模拟取消/失败：不变更入账金额，仍可继续付款。
- 模拟处理中：保持待确认，可稍后再次发起模拟成功，验证延迟确认的业务体验。

模拟支付仍需要客户登录、订单归属和组织权限。它不代替手机号授权，也不连接微信支付或 CloudBase。开发者工具无法完成真实支付验证。

后台打开订单详情查看通道和流水，使用 `payment.refund` 权限发起退款；本地 MOCK 退款不产生真实资金流。退款界面展示已退款和正在处理的预留金额。客户累计实付汇总排除 MOCK；订单本身保留模拟实付事实，便于验证履约。

需要停用本地模拟时：

```bash
PAYMENT_MOCK_ENABLED=false bash scripts/start-local.sh
```

环境变量在进程启动时读取；修改配置后需重启 API。模拟只允许 `APP_ENV=local` 和回环监听地址，使用 `:8080`、`0.0.0.0:8080` 或非 local 环境启用模拟将启动失败。不要将模拟服务通过代理、隧道或公网转发。

## 真机联调配置

真机开发版使用 `apps/wechat-mini/miniprogram/config/env.ts` 中的 `trial` HTTPS API 地址，体验版也使用该地址，正式版使用 `release` 地址。当前仓库仍是占位域名，必须替换为实际服务地址并在微信后台配置合法域名。`develop` 不能用作“开发者工具”的判断依据：手机运行开发版也走真实支付。

Go 环境配置：

| 配置 | 作用 |
| --- | --- |
| `APP_ENV` | 联调使用项目约定的非 local 环境标识 |
| `PAYMENT_MOCK_ENABLED=false` | 联调/正式强制关闭模拟 |
| `PAYMENT_WECHAT_ENABLED=true` | 开启真实通道及其回调、退款、查询能力 |
| `PAYMENT_COLLECTION_PAUSED=false` | 是否暂停新收款，启用联调时先保持暂停，完成部署后放开 |
| `PAYMENT_ORDER_TTL=15m` | 新订单有效期，必须大于零且不超过 24 小时 |
| `WECHAT_MINI_APP_ID` | 与客户登录和商户绑定的小程序 AppID 一致 |
| `PAYMENT_MERCHANT_ID` | 微信支付商户号 |
| `PAYMENT_CLOUD_ENV` | 云开发环境 ID |
| `PAYMENT_CLOUD_FUNCTION` | 集成生成并部署桥接代码的真实 HTTP 云函数名 |
| `PAYMENT_BRIDGE_URL` | Go 可访问的 HTTP 云函数 HTTPS 根地址，根路径映射到此函数 |
| `PAYMENT_BRIDGE_SECRET` | Go 和云函数共用的服务签名密钥，至少 32 字节 |
| `PAYMENT_TICKET_SECRET` | Go 独立持有的支付凭据密钥，至少 32 字节且与桥接密钥不同 |

云函数配置与部署材料位于 `deploy/cloudbase/payment-bridge`。使用集成中心托管的商户凭证，加上 Go HTTPS 根地址和桥接密钥。Go 与云函数必须配置相同的 AppID、商户号。集成中心自动生成的演示下单接口不能继续暴露并信任前端金额；本项目使用 `/wx-pay/prepay` 和短时支付凭据。

代码已提供请求/响应签名、预支付短时锁和预支付标识缓存。云函数每次都向 Go 校验订单状态；预支付成功后保存标识，再向客户端返回重新签名的收银台参数。结果未知时继续查询同一商户支付单号，不创建另一个有效收款尝试。

密钥从受控文件、进程环境或云控制台注入，不在聊天中发送，不提交到仓库；商户私钥/APIv3 密钥不放入小程序。正式配置还应满足项目已有登录、数据库 TLS、日志元数据及媒体存储要求；支付模块不替代这些部署要求。

## 通知与异常恢复

云函数收到通知后独立查询微信并验证渠道响应。Go 仅接收通过服务签名验证且商户、AppID、交易号和金额匹配的事件。支付事实、事件去重记录和审计在同一数据库事务内提交，成功提交后才返回确认。

每分钟任务处理到期的支付和退款查询，失败时退避重试。成功交易每 24 小时再次逐笔核对；这是逐笔 API 核对，不是微信账单文件下载对账。异常退款保留金额预留并持续查询，不能因申请失败提示就再次分配同一笔可退余额。

- 订单未创建渠道预支付：到期后本地取消。
- 渠道交易仍未支付：查单后关闭，再复查最终状态。
- 渠道查不到交易：有效期结束后额外等待 5 分钟，再以经过签名验证的不存在结果收敛为关闭。
- 微信已付款但客户端断网：通知或查询恢复后可在订单详情看到结果。
- 已取消订单收到真实付款：保留取消状态并入账，后台显示 `LATE_PAYMENT_REQUIRES_REFUND` 警示，由有权限人员核对退款。
- 退款返回受理：显示处理中，最终状态由可信通知或查询确定；不重复更换退款单号。
- `CHANNEL_CHECK_FAILED` / `REFUND_CHECK_FAILED`：检查桥接 URL、签名密钥、渠道响应验签与网络；数据库保留待处理事实，服务日志中的 `payment_reconcile` 任务会报告失败。

后台需配合现有日志告警设施关注支付任务失败、持续待确认、退款异常和迟到收款。仓库未新增或连接外部告警接收账号。

## 暂停新收款

设置 `PAYMENT_COLLECTION_PAUSED=true` 后重启 Go，保留 `PAYMENT_WECHAT_ENABLED=true`。新的支付单和预支付授权被拒绝，已有收银台付款结果、回调、查询、退款与核对继续处理。不要用关闭真实通道或回滚代码来代替停收款，否则可能中断在途交易恢复。

## 验收

```bash
npm run check
node --test scripts/test-mini-payment.cjs deploy/cloudbase/payment-bridge/test.cjs
go -C apps/server-go test ./...
go -C apps/server-go vet ./...
```

在独立且已执行所有迁移、名称以 `_test` 结尾的 PostgreSQL 数据库运行：

```bash
PAYMENT_TEST_DSN='postgres://USER:PASSWORD@HOST:PORT/DATABASE_test?sslmode=disable' \
  go -C apps/server-go test -race ./internal/payment -count=1 -v
```

自动化覆盖模拟成功/取消/失败/处理中、设备选择、禁用通道、过期、越权、签名篡改、重复支付、并发退款、历史订单豁免和付款后生成工单。仍需实际环境验证云函数路由、可信身份、商户绑定、通知重试与真实小额扣款/退款，并核对微信渠道与本地账。
