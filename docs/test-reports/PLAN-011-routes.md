# PLAN-011 客户私有路由清单

从当前 `internal/app/app.go` 登记，全部经过数据库客户会话中间件。唯一公开客户认证入口为 `POST /api/v1/mini/auth/phone/login`；无客户退出、切换账号或改号接口。

| 路由 | 处理器 | 身份与资源范围 |
| --- | --- | --- |
| `POST /api/v1/mini/auth/session/verify` | `customerAPI.Verify` | 当前会话组织及客户；目标资源另作归属校验 |
| `GET /api/v1/mini/auth/me` | `customerAPI.Me` | 当前会话组织及客户；目标资源另作归属校验 |
| `GET /api/v1/mini/profile` | `customerAPI.Profile` | 当前会话组织及客户；目标资源另作归属校验 |
| `PUT /api/v1/mini/profile` | `customerAPI.Update` | 当前会话组织及客户；目标资源另作归属校验 |
| `GET /api/v1/mini/checkout/contact` | `customerAPI.Contact` | 当前会话组织及客户；目标资源另作归属校验 |
| `POST /api/v1/mini/media/avatar` | `med.UploadAvatar` | 当前会话组织及客户；目标资源另作归属校验 |
| `POST /api/v1/mini/media/fault` | `med.UploadFault` | 当前会话组织及客户；目标资源另作归属校验 |
| `GET /api/v1/mini/media/{id}/content` | `med.Protected` | 当前会话组织及客户；目标资源另作归属校验 |
| `DELETE /api/v1/mini/media/{id}` | `med.Delete` | 当前会话组织及客户；目标资源另作归属校验 |
| `GET /api/v1/mini/cart` | `crt.Get` | 当前会话组织及客户；目标资源另作归属校验 |
| `POST /api/v1/mini/cart/items` | `crt.Add` | 当前会话组织及客户；目标资源另作归属校验 |
| `PATCH /api/v1/mini/cart/items/{id}` | `crt.Quantity` | 当前会话组织及客户；目标资源另作归属校验 |
| `PUT /api/v1/mini/cart/items/{id}/fault` | `crt.Fault` | 当前会话组织及客户；目标资源另作归属校验 |
| `DELETE /api/v1/mini/cart/items/{id}` | `crt.Delete` | 当前会话组织及客户；目标资源另作归属校验 |
| `POST /api/v1/mini/orders` | `ord.Create` | 当前会话组织及客户；目标资源另作归属校验 |
| `POST /api/v1/mini/orders/{id}/repeat` | `ord.Repeat` | 当前会话组织及客户；目标资源另作归属校验 |
| `GET /api/v1/mini/addresses` | `addr.List` | 当前会话组织及客户；目标资源另作归属校验 |
| `POST /api/v1/mini/addresses` | `addr.Create` | 当前会话组织及客户；目标资源另作归属校验 |
| `PUT /api/v1/mini/addresses/{id}` | `addr.Update` | 当前会话组织及客户；目标资源另作归属校验 |
| `POST /api/v1/mini/addresses/{id}/default` | `addr.SetDefault` | 当前会话组织及客户；目标资源另作归属校验 |
| `DELETE /api/v1/mini/addresses/{id}` | `addr.Delete` | 当前会话组织及客户；目标资源另作归属校验 |
| `GET /api/v1/mini/orders` | `ful.CustomerOrders` | 当前会话组织及客户；目标资源另作归属校验 |
| `GET /api/v1/mini/orders/{id}` | `ful.CustomerOrder` | 当前会话组织及客户；目标资源另作归属校验 |
| `GET /api/v1/mini/work-orders/{id}` | `ful.CustomerWorkOrder` | 当前会话组织及客户；目标资源另作归属校验 |
| `POST /api/v1/mini/work-orders/{id}/acceptance` | `ful.CustomerAcceptance` | 当前会话组织及客户；目标资源另作归属校验 |
| `POST /api/v1/mini/work-orders/{id}/rating` | `ful.SubmitRating` | 当前会话组织及客户；目标资源另作归属校验 |
| `GET /api/v1/mini/work-orders/{id}/timeline` | `ful.WorkOrderTimeline` | 当前会话组织及客户；目标资源另作归属校验 |

私有路由旧演示 Token 拒绝矩阵：`internal/app/app_test.go::TestAllCustomerRoutesRejectLegacyIdentity`。购物车、地址、订单、工单、评价、媒体及跨组织查询另见客户集成测试；请求参数不能替代会话身份。
