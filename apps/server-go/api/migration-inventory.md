# Go 迁移接口清单

外部契约统一使用 `/api/v1`，响应外壳为 `{code,message,data,requestId}`；业务 ID 为字符串，金额为整数分，时间为 UTC RFC3339。

- Public：健康检查、Ping、公开媒体、分类目录、服务搜索和服务详情。
- Admin Basic：分类增改启停、SKU 分页/详情/增改/发布/下架、SKU 图片、订单分页和详情。
- Mini Bearer：故障媒体、购物车查询/加购/数量/资料/删除、幂等创建订单。
- 媒体：SKU 图片最大 10 MB；故障图片最大 10 MB、视频最大 50 MB；文件签名校验；私有媒体校验 owner。
- 订单：`Idempotency-Key` + SHA-256 请求摘要 + PostgreSQL 唯一键；订单金额、SKU 和客户身份均由服务端确定。

路由注册的唯一事实来源为 `internal/app/app.go`，前端调用源分别为 `apps/admin-web/src/api` 和 `apps/wechat-mini/miniprogram/services`。
