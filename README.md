# 灵狸护家 县域家居水电运维平台

当前工程采用 Go 模块化单体后端、React 管理后台和微信原生 TypeScript 小程序。

## 工程结构

```text
apps/
├─ server-go/    Go 1.26 后端
├─ admin-web/    React + TypeScript 管理后台
└─ wechat-mini/  微信原生 TypeScript 小程序
deploy/
└─ compose.yaml  本地 Go 后端、PostgreSQL、Redis、MinIO
```

## 环境要求

- Go 1.26+
- Node.js 24+
- Docker Desktop / Docker Engine + Compose（推荐）
- 微信开发者工具

## 一键启动后端环境

本机快速启动步骤见 [本地部署与快速启动手册](deploy_local.md)。

```powershell
docker compose -f deploy/compose.yaml up -d --build postgres migrate server
```

访问：

- 健康检查：`http://localhost:8080/actuator/health`
- Ping：`http://localhost:8080/api/v1/public/ping`
- 管理员首次使用：先执行 `go run ./cmd/bootstrap-admin -org-id 1 -username admin -platform-super-admin`，默认账号/密码为 `admin / 12345678`，登录后必须改密；可通过 `BOOTSTRAP_ADMIN_PASSWORD` 或 `-password` 覆盖

云端使用本项目 Docker 镜像时，先完成数据库迁移，再在运行中的后端容器初始化管理员：

```bash
docker compose -f deploy/compose.yaml exec -T server /app/bootstrap-admin -org-id 1 -username admin -platform-super-admin
```

初始化无需交互终端，默认密码为 `12345678`，密码不会写入日志。优先级为 `-password` → `BOOTSTRAP_ADMIN_PASSWORD` → `APP_ADMIN_PASSWORD` → 默认值。普通重启不会重置密码；已有数据库的密码不会因升级镜像自动改变，显式重新执行初始化命令会重置同名账号并要求首次登录改密。自定义 Compose 项目需使用实际配置文件或容器名执行该命令。
- 客户小程序：微信手机号快捷登录，已移除演示令牌

## 本机启动 Go 后端

先启动 PostgreSQL，然后在 PowerShell 设置环境变量：

```powershell
cd apps/server-go
$env:DB_DSN='postgres://fixpro:fixpro-local@localhost:5433/fix_pro?sslmode=disable&timezone=UTC'
go run ./cmd/migrate
go tool air -c .air.toml
```

Air 仅用于本地开发，会监听 Go 源码并自动重新构建、重启服务；不需要全局安装 `air`。也可以在 `apps/server-go` 下执行 `make dev`。`.env.example` 是配置清单，Go 程序和 Air 都不会隐式读取 `.env`；请使用 PowerShell、IDE Run Configuration 或 Compose 注入变量。

修改数据库迁移 SQL 不会触发迁移，仍需显式执行 `go run ./cmd/migrate`。生产环境和 Docker 继续使用编译后的 `/app/server`，不依赖 Air。若不需要热重载，仍可使用 `go run ./cmd/server`。

## 前端

```powershell
npm install
npm run admin:dev
npm run check
```

管理后台访问 `http://localhost:5173`，Vite 会把 `/api` 代理到 `http://localhost:8080`。小程序使用微信开发者工具导入 `apps/wechat-mini`。

后台使用 HttpOnly Cookie 会话和 CSRF Token，不再把 Basic 凭据保存到浏览器；权限、角色和用户管理入口位于“角色与权限”和“后台用户”。

## 后端质量命令

```powershell
cd apps/server-go
gofmt -w .
go vet ./...
go test ./...
go build ./cmd/server ./cmd/migrate
```

正向链路和接口级验证步骤见 [本地开发与正向链路验证手册](docs/runbooks/local-development.md)。数据库迁移设计与执行清单见 [SPEC-003](docs/specs/SPEC-003-MySQL迁移PostgreSQL.md) 和 [PLAN-003](docs/plans/PLAN-003-MySQL迁移PostgreSQL.md)。

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

后台 `/customers` 为“用户管理”，独立权限为 `customer.view`，内置租户管理员自动获得；自定义角色需分配。查看最近订单还需 `order.view`。“后台用户”是管理员账号模块。注册活跃统计以当前组织、北京时间自然日及服务端截止时间计算；总体统计不随列表筛选变化。订单汇总按订单事实聚合，金额为 `paid_amount` 分，展示为人民币；不提供客户开户、禁用、改号、导出或代登录。

验证记录见 `docs/test-reports/PLAN-011-e2e.md`。真实微信授权、真机两号隔离和跨环境恢复须单独验收，网关测试桩不能替代。
