# CloudBase 支付桥接函数

与 Go 支付模块配套使用的 HTTP 云函数源码。它保留教程中的集成中心、凭证配置和小程序 `callHTTPFunction` 调用方式，但替换演示用下单入口，拒绝客户端决定金额和 OpenID。仅使用 Node 内置模块，无 npm 安装步骤。

## 部署前

1. 按 [CloudBase 教程](https://docs.cloudbase.net/integration/wechat-pay-miniprogram) 完成商户与小程序绑定，创建集成，记录真实云函数名和回调 URL。
2. 使用同一 AppID/商户号配置 Go 与云函数。Go 的 `PAYMENT_BRIDGE_SECRET` 与函数一致；Go 的 `PAYMENT_TICKET_SECRET` 独立生成且不放入函数或客户端。
3. 云函数配置商户凭证及 `PAYMENT_BACKEND_URL`（Go 的 HTTPS 根地址，不包含 `/api`）；配置项见 `cloudbaserc.json`。凭证从受控环境或控制台注入，不提交实际值。
4. 向集成生成的实际函数部署本目录内容。`cloudbaserc.json` 使用环境占位符；部署前确认 CLI 已正确解析函数名与环境 ID。若控制台编辑部署，保留集成环境变量，上传本目录的 `index.js`、`index.cjs`、`package.json`。
5. HTTP 云函数启动进程监听 `PORT`，默认 `9000`。可用 `npm start` 或容器 Dockerfile 运行同一实现；小程序调用需要 HTTP 云函数名。
6. 配置 HTTP 访问路由，使根路径下的 `/internal/query` 和 `/internal/refund` 可被 Go 调用；它们均要求 HMAC。`PAYMENT_BRIDGE_URL` 必须是 HTTPS 根地址，并配置根路径路由到该函数；不接受额外路径前缀，避免服务签名与实际路径不一致。实际部署验收必须验证这一点。
7. 核对集成中心转发到本函数的真实路由及明文对象结构。支持 `/wechatpay/order`、`/wechatpay/refund`、`/wx-pay/unifiedOrderTrigger`、`/wx-pay/refundTrigger`；如控制台生成其他路由，先明确映射再部署。

这里没有执行云资源部署，也没有用真实商户做端到端验证；CLI 模板和默认路由需要在实际环境验收。

## 接口

- 小程序 `POST /wx-pay/prepay`：仅提交 Go 签发的短时 `ticket`；业务参数由签名服务请求向 Go 获取。
- Go `POST /internal/query`：查单，过期且未支付时关单并复查。Go 和函数双向校验请求/响应签名。
- Go `POST /internal/refund`：先按退款单号查询；渠道无该单才申请退款，始终复用单号。
- 支付/退款通知：加密原始通知先验签解密；集成中心明文通知只作为查单提示。两者都独立查微信支付并验证响应签名，再将真实查询结果提交 Go。公网伪造的成功状态和金额不能直接入账。

响应签名使用微信支付公钥 ID 与 RSA 公钥验证，不接受任意序列号。`REFUND` 交易状态保留为已付款事实，退款金额由退款单独立记录。所有微信请求超时或未知结果均由 Go 的可靠记录和定时查询恢复。

## 本地验证

```bash
node --test deploy/cloudbase/payment-bridge/test.cjs
```

测试用临时 RSA 密钥与替身响应，不访问真实支付渠道。完整启用步骤及验收见 `docs/runbooks/payment.md`。
