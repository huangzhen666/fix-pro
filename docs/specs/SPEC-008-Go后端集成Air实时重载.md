# SPEC-008｜Go 后端集成 Air 实时重载

**状态：** Draft  
**版本：** V1.0  
**日期：** 2026-08-12  
**适用工程：** `apps/server-go`  
**关联文档：** `docs/runbooks/local-development.md`、`README.md`

## 1. 背景与问题

当前 Go 后端通过 `go run ./cmd/server` 或编译后的可执行文件启动。修改任意 Go 文件后，需要手动停止进程、重新编译并重新启动，降低了本地开发效率，也容易出现旧进程未退出、端口被占用和运行产物不一致的问题。

Air 是 Go 应用的本地实时重载工具。官方文档明确说明，Air 用于开发阶段的代码变更监听、重新构建和重启，不用于生产环境热部署；官方当前推荐使用 `go install github.com/air-verse/air@latest`，或使用 Go 1.25+ 的项目内工具方式 `go get -tool github.com/air-verse/air@latest` 后通过 `go tool air` 启动。当前项目使用 Go 1.26，因此采用项目内工具方式。

## 2. 目标与非目标

### 2.1 目标

1. 在 `apps/server-go` 目录执行一个统一命令即可启动带实时重载的 Go 后端。
2. 修改后端 Go 源码后，Air 自动重新构建并重启 `cmd/server`。
3. Windows、Linux/WSL 和 macOS 使用同一份配置，并按平台选择正确的可执行文件路径。
4. Air 版本进入 Go 模块工具依赖，团队成员和 CI 使用同一版本。
5. 保留现有手动启动、数据库迁移、生产构建和 Docker 启动方式，不改变业务运行行为。
6. 失败构建、端口冲突、进程退出和恢复过程都有清晰日志。

### 2.2 非目标

- 不在生产环境启用 Air。
- 不把 Air 打进生产 Docker 镜像。
- 不在每次 Air 重启时自动执行数据库迁移。
- 不为 React 管理后台或微信小程序增加前端热重载配置；前端继续使用各自的开发工具。
- 不改变 HTTP API、数据库模型、认证方式和业务状态机。

## 3. 当前工程基线

| 项目 | 当前约定 |
| --- | --- |
| Go 模块 | `github.com/fixpro/server` |
| Go 版本 | `1.26.0` 及以上 |
| 服务入口 | `apps/server-go/cmd/server` |
| 本地服务端口 | `:8080` |
| 数据库 | PostgreSQL，由 `DB_DSN` 注入 |
| 数据库迁移 | `go run ./cmd/migrate`，单独执行 |
| 现有手动启动 | `go run ./cmd/server` |
| 生产启动 | Docker 镜像中的 `/app/server` |
| 开发构建产物 | `apps/server-go/bin/`，该目录已被 Git 忽略 |

## 4. 核心设计决策

### 4.1 使用项目内 Air 工具

在 `apps/server-go/go.mod` 中使用 Go 工具依赖声明，执行：

```powershell
cd apps/server-go
go get -tool github.com/air-verse/air@latest
go tool air -v
```

实际执行后必须将解析到的 Air 版本锁定在 `go.mod`/`go.sum` 中，不允许提交依赖 `PATH` 中全局 Air 的方案。开发者不需要单独维护 GOPATH 下的 Air 版本。

### 4.2 Air 仅负责服务进程

Air 的构建目标固定为 `./cmd/server`。`cmd/migrate` 不纳入 Air 的启动链路，迁移仍由开发者在启动服务前显式执行：

```powershell
go run ./cmd/migrate
go tool air -c .air.toml
```

这样可以避免每次保存源码都重复执行迁移，也避免开发环境误修改数据库结构。

### 4.3 使用 `entrypoint`，不使用已弃用的 `bin` 字段

Air 配置使用 `build.entrypoint` 指向构建产物。Windows 使用 `.exe`，Linux/WSL 和 macOS 使用无扩展名二进制。配置必须排除 `bin`、`tmp`、`vendor` 和测试数据目录，避免 Air 监听自己生成的文件而进入重载循环。

## 5. 功能需求

### FR-01｜项目内安装与版本锁定

1. `apps/server-go/go.mod` 增加 Air 工具依赖声明。
2. `go.sum` 记录完整校验信息。
3. `go tool air -v` 能输出版本并正常退出。
4. 新开发环境只需满足 Go 版本要求并执行模块依赖下载，即可使用 Air；不要求预装全局 `air` 命令。

### FR-02｜统一开发启动命令

在 `apps/server-go/Makefile` 增加：

```make
dev:
	go tool air -c .air.toml
```

同时支持直接执行：

```powershell
cd apps/server-go
go tool air -c .air.toml
```

现有 `make run`/`go run ./cmd/server` 继续保留，作为不使用 Air 时的最小启动方式。

### FR-03｜源码监听范围

默认监听 `apps/server-go` 下的 Go 源码文件，至少覆盖：

- `cmd/**/*.go`
- `internal/**/*.go`
- `api/**/*.go`（如后续生成或维护 Go 文件）
- `test/**/*.go` 不触发服务重启

以下内容不触发服务重启：

- `bin/`、`tmp/`、`vendor/`、`testdata/`
- `*_test.go`
- `docs/`、Markdown、图片和其他仓库文档
- `db/migrations/*.sql`；迁移文件变更必须通过 `cmd/migrate` 单独验证

第一期只监听 Go 文件，避免把“修改环境变量但未真正注入进程”误认为已生效。若未来后端增加运行时模板或静态资源，再单独增加 Air rule，并为该规则定义对应命令。

### FR-04｜构建配置

根目录配置文件为 `apps/server-go/.air.toml`，要求：

- 构建命令：`go build -o ./bin/fixpro-server-dev ./cmd/server`
- Windows 构建产物：`./bin/fixpro-server-dev.exe`
- Linux/WSL/macOS 构建产物：`./bin/fixpro-server-dev`
- 构建失败时停止启动新进程，并保留错误日志供修复后自动重试
- 默认防抖时间为约 1 秒，避免编辑器连续写入造成重复构建
- 在支持的平台启动时优先发送中断信号，并设置有限的 kill delay；Windows 按 Air 官方行为使用 `TASKKILL` 终止旧进程
- Air 启动横幅可关闭，业务日志格式仍由服务端自身控制

建议配置形态如下，具体字段以安装时锁定的 Air 版本校验为准：

```toml
root = "."

[build]
cmd = "go build -o ./bin/fixpro-server-dev ./cmd/server"
entrypoint = ["./bin/fixpro-server-dev"]
include_ext = ["go"]
exclude_dir = ["bin", "tmp", "vendor", "testdata"]
exclude_regex = ["_test\\.go$"]
delay = 1000
stop_on_error = true
send_interrupt = false
kill_delay = "500ms"

[build.windows]
cmd = "go build -o ./bin/fixpro-server-dev.exe ./cmd/server"
entrypoint = ["bin\\fixpro-server-dev.exe"]

[misc]
startup_banner = ""
```

### FR-05｜环境变量与本地配置

1. Air 不绕过现有配置加载逻辑；`HTTP_ADDR`、`DB_DSN`、媒体目录、管理员账号等仍由进程环境变量提供。
2. 不把真实密码、数据库连接串或媒体密钥写入 `.air.toml`。
3. `.env.example` 继续只作为配置清单；是否加载 `.env` 不由 Air 默认隐式改变。
4. Windows PowerShell、IDE 或 Compose 注入的环境变量必须在 Air 启动前生效。
5. 如后续需要 Air 自动加载开发环境文件，新增 `.env.air.example`，并明确该文件只允许本地开发值；真实 `.env.air` 必须被 Git 忽略。

### FR-06｜优雅重启与端口行为

1. Linux/macOS 重启前优先发送中断信号，使现有 `http.Server.Shutdown` 执行；Windows 按 Air 官方实现使用 `TASKKILL`。
2. 旧进程在有限时间内未退出时，Air 才执行强制终止。
3. 新进程仍监听 `HTTP_ADDR`，默认保持 `:8080`，不新增代理端口。
4. 若数据库不可用、配置不合法或端口被占用，Air 必须显示原始错误；修复源码或外部依赖后可以再次自动构建/启动。

### FR-07｜调试模式

开发者可使用：

```powershell
go tool air -c .air.toml -d
```

该模式用于排查“为什么没有触发重载”，不作为日常默认启动方式。

## 6. 预期工程变更

实施本 Spec 时只允许修改以下范围：

1. `apps/server-go/go.mod`
2. `apps/server-go/go.sum`
3. `apps/server-go/.air.toml`
4. `apps/server-go/Makefile`
5. 根目录 `README.md` 的本地后端启动说明
6. `docs/runbooks/local-development.md` 的 Air 启动和排障说明
7. 必要时更新 `.gitignore`，确保 Air 生成的开发二进制不被提交

不得为了接入 Air 修改业务代码、数据库迁移、API 契约、Docker 生产入口或前端代码。

## 7. 验收标准

### AC-01｜工具可用

在干净环境中执行 `go mod download` 后，以下命令成功：

```powershell
cd apps/server-go
go tool air -v
```

### AC-02｜首次启动

PostgreSQL 已启动、环境变量已配置时执行 `go tool air -c .air.toml`，服务启动成功，并且：

- `GET http://localhost:8080/actuator/health` 返回 HTTP 200；
- `GET http://localhost:8080/api/v1/public/ping` 返回 HTTP 200；
- 运行目录没有生成未被忽略的临时构建文件。

### AC-03｜Go 源码自动重载

修改一个不会改变业务结果的 Go 文件并保存，Air 在防抖和构建完成后重新启动服务。通过服务启动日志或请求结果确认新进程已生效，目标响应时间不超过 5 秒（不含首次下载依赖）。

### AC-04｜构建失败恢复

临时引入语法错误，Air 输出构建失败且不启动损坏产物；修复语法后，Air 自动重新构建并恢复服务。

### AC-05｜无关文件不重启

修改 Markdown、图片、`*_test.go`、`db/migrations/*.sql` 和 `bin/` 下生成文件，不触发服务二进制重建。迁移 SQL 仍需执行独立的 `go run ./cmd/migrate` 才生效。

### AC-06｜平台兼容

- Windows PowerShell 能生成并运行 `bin/fixpro-server-dev.exe`；
- Linux/WSL 能生成并运行 `bin/fixpro-server-dev`；
- 两个平台均使用同一个 `HTTP_ADDR` 和数据库配置约定。

### AC-07｜生产隔离

以下命令不依赖 Air 且通过：

```powershell
go build ./cmd/server ./cmd/migrate
docker build -t fixpro-server-go:local .
```

构建出的生产镜像中不存在 Air 工具和开发二进制，容器入口仍为 `/app/server`。

### AC-08｜质量门禁

```powershell
gofmt -w .
go vet ./...
go test ./...
```

全部通过，且 `git status` 不包含 Air 生成的二进制、日志或本地环境文件。

## 8. 排障约定

| 现象 | 优先检查 |
| --- | --- |
| `go tool air` 找不到 | 当前目录是否为 `apps/server-go`，Go 是否为 1.26+，工具依赖是否已下载 |
| 修改代码没有重启 | 是否修改了 Go 文件；使用 `-d` 查看监听和匹配日志 |
| 端口占用 | 检查旧的 `go run`/可执行文件进程，确认只有一个服务实例监听 8080 |
| 数据库连接失败 | 先确认 PostgreSQL 和 `DB_DSN`，不要把数据库迁移交给 Air |
| Windows 无法执行产物 | 检查 `.air.toml` 的 Windows `entrypoint` 和 `bin\\\\fixpro-server-dev.exe` 路径 |
| 修改 SQL 后接口未变化 | 这是预期行为；先停止/保持服务，再执行 `go run ./cmd/migrate` |

## 9. 风险与后续演进

1. **版本升级风险：** Air 使用最新版本安装时可能调整配置字段；实现时必须以锁定版本的 `air -h` 和示例配置校验 `.air.toml`。
2. **Windows 文件锁：** 某些编辑器或杀毒软件可能短暂锁定 `.exe`，需要通过 `kill_delay` 或更换开发输出文件名处理，不能修改业务启动逻辑。
3. **依赖下载：** 首次 `go get -tool`/`go mod download` 需要网络；团队可通过 Go 模块缓存或内部代理加速，但不能提交 Air 二进制到仓库。
4. **未来生成代码：** 若引入 `sqlc`、`templ` 或前端静态资源生成，应使用 Air 的独立 `build.rules`，并确保规则不会与主构建形成循环触发。

## 10. 完成定义

当项目内 Air 工具版本已锁定、`.air.toml` 已提交、Windows 与 Linux/WSL 至少各完成一次自动重载验收、生产构建保持不依赖 Air、文档和排障命令已更新，并且 AC-01 至 AC-08 全部通过时，本 Spec 才算完成。

## 11. 参考资料

- Air 官方中文文档：<https://github.com/air-verse/air/blob/master/README-zh_cn.md>
- Air 官方配置示例：<https://github.com/air-verse/air/blob/master/air_example.toml>
