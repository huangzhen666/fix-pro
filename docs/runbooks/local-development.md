# 本地开发与正向链路验证

## 1. 启动后端与 PostgreSQL

推荐使用 Compose：

```powershell
docker compose -f deploy/compose.yaml up -d --build postgres migrate server
curl.exe http://localhost:8080/actuator/health
curl.exe http://localhost:8080/api/v1/public/ping
```

也可本机运行：

```powershell
cd apps/server-go
$env:DB_DSN='postgres://fixpro:fixpro-local@localhost:5432/fix_pro?sslmode=disable&timezone=UTC'
go run ./cmd/migrate
go run ./cmd/server
```

迁移命令使用 golang-migrate。已执行的 `up.sql` 不应修改，表结构变化应新增下一版本迁移。

## 2. 验证管理端 SKU 正向链路

1. 启动管理后台：`npm run admin:dev`。
2. 使用 `admin / change-me-in-production` 登录。
3. 在“服务分类”新建或选择分类。
4. 在“服务 SKU”上传封面图，填写固定价格、服务范围、除外项、质保说明并保存。
5. 发布 SKU，确认列表状态为 `PUBLISHED`。

## 3. 验证小程序购物车与下单

1. 用微信开发者工具导入 `apps/wechat-mini`，本地环境令牌为 `Bearer local-customer-1`。
2. 在“全部服务”确认刚发布的 SKU 和封面图可见。
3. 进入详情并加入购物车。
4. 上传至少一张故障图片或一个视频，填写 5—500 字故障描述。
5. 填写联系人、11 位手机号及服务地址后提交订单。
6. 重复相同请求和 `Idempotency-Key` 应返回同一订单；相同键配不同请求应返回 `ORDER_SUBMIT_DUPLICATED`。
7. 回到管理后台订单列表，确认订单及故障资料可见。

## 4. API 快速检查

```powershell
curl.exe -u admin:change-me-in-production http://localhost:8080/api/v1/admin/catalog/categories?includeDisabled=true
curl.exe http://localhost:8080/api/v1/catalog/categories
curl.exe -H "Authorization: Bearer local-customer-1" http://localhost:8080/api/v1/mini/cart
curl.exe -u admin:change-me-in-production http://localhost:8080/api/v1/admin/orders?page=1&pageSize=20
```

所有 JSON 接口保持 `{code,message,data,requestId}` 响应外壳，业务 ID 为字符串，金额单位为分。

## 5. 提交前检查

```powershell
npm run check
cd apps/server-go
gofmt -w .
go vet ./...
go test ./...
go build ./cmd/server ./cmd/migrate
```

生产环境不得使用默认管理员密码、本地客户令牌或本地媒体存储。需要在上线前接入正式身份认证和对象存储。
