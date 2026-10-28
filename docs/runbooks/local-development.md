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
$env:DB_DSN='postgres://fixpro:fixpro-local@localhost:5433/fix_pro?sslmode=disable&timezone=UTC'
go run ./cmd/migrate
go tool air -c .air.toml
```

Air 是项目内锁定版本的本地开发热重载工具，不需要全局安装 `air`。也可以执行 `make dev`。Air 只监听 Go 源码，修改后会自动构建并重启 `cmd/server`；不需要热重载时使用 `go run ./cmd/server`。

迁移命令使用 golang-migrate。已执行的 `up.sql` 不应修改，表结构变化应新增下一版本迁移。修改迁移 SQL 不会由 Air 自动执行，必须显式再次运行 `go run ./cmd/migrate`。

### 1.1 Air 调试和排障

```powershell
go tool air -c .air.toml -d
```

- Air 生成的开发二进制位于 `apps/server-go/.tmp/air/`，已被 Git 忽略，停止 Air 时会按配置清理。
- 如果 `go tool air` 找不到，先在 `apps/server-go` 执行 `go mod download`，并确认 Go 版本为 1.26+。
- 如果修改 Go 文件没有重启，使用 `-d` 查看监听日志；Markdown、图片、`*_test.go`、迁移 SQL 和构建产物不会触发主服务重建。
- 如果端口 8080 被占用，先停止旧的 `go run`、服务 exe 或 Docker server 容器，确保只运行一个后端实例。
- Windows 使用 `.tmp/air/fixpro-server-dev.exe`；Linux/WSL/macOS 使用 `.tmp/air/fixpro-server-dev`，两者共享同一份 `.air.toml`。

## 2. 验证管理端 SKU 正向链路

1. 启动管理后台：`npm run admin:dev`。
2. 使用 `admin / 12345678` 登录。
3. 在“服务分类”新建或选择分类。
4. 在“服务 SKU”上传封面图，填写固定价格、服务范围、除外项、质保说明并保存。
5. 发布 SKU，确认列表状态为 `PUBLISHED`。

## 3. 验证小程序购物车与下单

1. 用微信开发者工具导入 `apps/wechat-mini`，使用微信手机号快捷登录。
2. 在“全部服务”确认刚发布的 SKU 和封面图可见。
3. 进入详情并加入购物车。
4. 上传至少一张故障图片或一个视频，填写 5—500 字故障描述。
5. 填写联系人、11 位手机号及服务地址后提交订单。
6. 重复相同请求和 `Idempotency-Key` 应返回同一订单；相同键配不同请求应返回 `ORDER_SUBMIT_DUPLICATED`。
7. 回到管理后台订单列表，确认订单及故障资料可见。

## 4. API 快速检查

```powershell
curl.exe -u admin:12345678 http://localhost:8080/api/v1/admin/catalog/categories?includeDisabled=true
curl.exe http://localhost:8080/api/v1/catalog/categories
curl.exe -H "Authorization: Bearer <手机号登录返回的accessToken>" http://localhost:8080/api/v1/mini/cart
curl.exe -u admin:12345678 http://localhost:8080/api/v1/admin/orders?page=1&pageSize=20
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

管理员初始化默认密码为 `12345678`，首次登录必须修改；生产环境不得使用本地客户令牌或本地媒体存储，需要接入正式身份认证和对象存储。

## PLAN-011 客户手机号登录与用户管理

客户演示 Token 已移除。游客仍可浏览服务；购物车、地址、订单和资料编辑要求主动使用微信手机号快捷登录。小程序不提供退出、切换账号或改号入口。

### macOS 本地启动

```bash
bash scripts/start-local.sh
# 新增迁移后显式执行：
bash scripts/start-local.sh --migrate
```

服务端配置保存于根目录 `.tmp/customer-auth.env`（已被 Git 忽略，权限建议 `600`），启动脚本注入 `WECHAT_MINI_APP_ID`、`WECHAT_MINI_APP_SECRET`、`CUSTOMER_DEFAULT_ORG_ID=1`、`CUSTOMER_SESSION_TTL=168h`、`CUSTOMER_MOBILE_ENCRYPTION_KEY`、`CUSTOMER_MOBILE_HASH_KEY`。两套手机号密钥分别使用独立随机 32 字节的 Base64 编码；必须保留，重启时不能重新生成。不要把 AppSecret 放入 `miniprogram/config/env.ts` 或提交到仓库。Go 不自动读取 env 文件，手动启动需要自行注入上述变量。

### 首次初始化与日常启动

所有数据库先执行结构迁移，再暂停后端及客户/工单写入，进入 `apps/server-go`，在同一服务端环境配置中执行：

```bash
go run ./cmd/init-customer-auth             # 只输出目标数据库、计数及媒体清单
go run ./cmd/init-customer-auth -execute    # 一次性删除历史客户测试数据并收口手机号必填约束
# 若数据库已提交但私有媒体删除失败：
go run ./cmd/init-customer-auth -retry-media
```

清理保留组织、后台管理员、师傅档案、服务目录及被保留资源引用的媒体。初始化标记防止再次整批删除；日常重启和 migrate 不清空客户数据。首次初始化没有完成时手机号登录返回 `PHONE_LOGIN_UNAVAILABLE`。

本机 `fix_pro` 已于 2026-10-06 完成初始化，清单为 `.tmp/customer-auth-init-preview.json` 和 `.tmp/customer-auth-init-executed.json`，日常直接使用启动脚本。

### 小程序及后台

用微信开发者工具导入 `apps/wechat-mini`，开发环境 API 为 `http://localhost:8080`。手机号授权需要微信后台已开通能力且有额度；真机使用实际可访问的 HTTPS 服务及请求/上传/下载合法域名，trial/release 域名仍需配置。微信后台还需维护正式用户协议、隐私保护指引与收集说明。点击“我的”头像编辑头像昵称，资料仅在保存时提交。

地址表单的服务城市固定为汉川市，“定位”使用 `wx.chooseLocation` 打开微信地图，并回填街道及地点名称；楼号/门牌号单独填写。`app.json` 已声明 `chooseLocation` 和位置用途。上线前在微信后台「开发管理 → 接口设置」确认已开通选择地理位置接口，并在隐私保护指引中补充位置信息用途。真机验证首次隐私授权、位置权限拒绝、取消选址、汉川市内回填和跨城提示；未开通或用户拒绝时仍可手动填写。

后台 `/customers` 为“用户管理”，独立权限为 `customer.view`，内置租户管理员自动获得；自定义角色需分配。查看最近订单还需 `order.view`。“后台用户”是管理员账号模块。注册活跃统计以当前组织、北京时间自然日及服务端截止时间计算；总体统计不随列表筛选变化。订单汇总按订单事实聚合，金额为 `paid_amount` 分，展示为人民币；不提供客户开户、禁用、改号、导出或代登录。

验证记录见 `docs/test-reports/PLAN-011-e2e.md`。真实微信授权、真机两号隔离和跨环境恢复须单独验收，网关测试桩不能替代。
