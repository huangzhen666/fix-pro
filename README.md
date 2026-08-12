# FixPro 县域家居水电运维平台

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

```powershell
docker compose -f deploy/compose.yaml up -d --build postgres migrate server
```

访问：

- 健康检查：`http://localhost:8080/actuator/health`
- Ping：`http://localhost:8080/api/v1/public/ping`
- 管理员本地账号：`admin / change-me-in-production`
- 小程序本地令牌：`Bearer local-customer-1`

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

## 后端质量命令

```powershell
cd apps/server-go
gofmt -w .
go vet ./...
go test ./...
go build ./cmd/server ./cmd/migrate
```

正向链路和接口级验证步骤见 [本地开发与正向链路验证手册](docs/runbooks/local-development.md)。数据库迁移设计与执行清单见 [SPEC-003](docs/specs/SPEC-003-MySQL迁移PostgreSQL.md) 和 [PLAN-003](docs/plans/PLAN-003-MySQL迁移PostgreSQL.md)。
