# PLAN-008｜Go 后端集成 Air 实时重载实施计划

**状态：** Planned  
**版本：** V1.0  
**日期：** 2026-08-12  
**对应 Spec：** [SPEC-008-Go后端集成Air实时重载.md](../specs/SPEC-008-Go后端集成Air实时重载.md)  
**适用工程：** `apps/server-go`

## 1. 交付目标

在不改变业务代码、API、数据库模型和生产启动方式的前提下，为 Go 后端增加项目内 Air 开发工具：

```text
PostgreSQL/迁移准备
→ go tool air 启动
→ 修改 Go 源码
→ Air 自动构建
→ 优雅停止旧进程
→ 启动新进程
→ 健康接口可用
```

最终开发者在 `apps/server-go` 下可以执行：

```powershell
go tool air -c .air.toml
```

或：

```powershell
make dev
```

生产 Docker 继续使用 `/app/server`，不依赖 Air。

## 2. 实施原则

1. Air 只用于本地开发，不进入生产运行链路。
2. 使用 Go 项目内工具依赖锁定 Air 版本，不依赖开发者全局 PATH 中的 `air`。
3. Air 只构建和运行 `cmd/server`，不自动执行数据库迁移。
4. 所有迁移仍通过 `go run ./cmd/migrate` 显式执行。
5. 修改范围严格限制在工具依赖、Air 配置、开发命令和文档。
6. 先完成配置静态校验，再做 Windows 自动重载；Linux/WSL 验收具备环境时执行。
7. 不提交 Air 二进制、开发构建产物、日志或本地环境文件。
8. 每个里程碑完成验证后再进入下一个里程碑。

## 3. 当前基线

| 项目 | 当前值 |
| --- | --- |
| Go 模块 | `github.com/fixpro/server` |
| Go 版本 | `1.26.0` |
| 服务入口 | `apps/server-go/cmd/server` |
| 数据库迁移入口 | `apps/server-go/cmd/migrate` |
| 本地端口 | `:8080` |
| 当前开发启动 | `go run ./cmd/server` |
| 生产启动 | Docker `/app/server` |
| 可忽略目录 | `apps/server-go/bin/`、`.tmp/` |

## 4. 里程碑总览

| 里程碑 | 交付结果 | 主要验证 | 依赖 |
| --- | --- | --- | --- |
| P0 | 依赖和版本策略确认 | Go 版本、网络、工具命令可用 | 无 |
| P1 | Air 工具依赖锁定 | `go tool air -v` 成功 | P0 |
| P2 | `.air.toml` 构建与监听配置 | 配置可解析、平台入口正确 | P1 |
| P3 | Makefile 和开发文档接入 | 一条命令启动 | P2 |
| P4 | Windows 本地自动重载 | 首次启动、源码修改、失败恢复通过 | P3 |
| P5 | Linux/WSL 兼容验证 | 无扩展名二进制可运行 | P4，环境可用 |
| P6 | 生产隔离和质量门禁 | Go 构建、Docker、测试不依赖 Air | P3 |
| P7 | 收尾和交付 | 文档、Git 状态、回滚信息完整 | P4-P6 |

关键路径：

```text
P0 → P1 → P2 → P3 ─┬→ P4 → P5
                    └→ P6 → P7
```

## 5. P0｜环境与实现边界确认

### P0-1｜确认 Go 工具能力

检查：

```powershell
go version
go env GOMOD GOPATH GOBIN
go help get
go help tool
```

验收：

- Go 版本满足 `1.26.0+`；
- 当前工作目录为 `apps/server-go` 时能够识别 `go.mod`；
- `go get -tool` 和 `go tool` 命令可用。

如果本机 Go 版本低于要求，不修改项目版本以迁就本机，先升级开发环境。

### P0-2｜确认开发依赖

确认：

- PostgreSQL 已启动，`DB_DSN` 可连接；
- 8080 端口没有被旧的 `go run`、旧 exe 或 Docker 服务占用；
- PowerShell 可执行 `go build ./cmd/server`；
- 如要进行 WSL 验收，WSL 内可访问项目和 PostgreSQL。

验收：记录环境问题，但不把数据库、前端或 Docker 重构纳入本计划。

### P0-3｜冻结修改边界

实施前记录：

```powershell
git status --short
git diff -- apps/server-go README.md docs/runbooks
```

退出条件：确认已有用户修改不被覆盖，后续只修改 Spec 允许的文件。

## 6. P1｜项目内 Air 依赖

### P1-1｜添加工具依赖

在 `apps/server-go` 执行：

```powershell
go get -tool github.com/air-verse/air@latest
go mod tidy
```

要求：

- Air 进入 Go 工具依赖声明；
- `go.sum` 有完整校验；
- 不执行 `go install` 作为项目集成方式；
- 不提交下载到 GOPATH 的二进制。

### P1-2｜确认锁定版本和命令

执行：

```powershell
go tool air -v
go list -m all
git diff -- go.mod go.sum
```

验收：

- `go tool air -v` 返回版本；
- 依赖 diff 仅包含 Air 及其必要的模块调整；
- 不出现无关业务依赖升级。

回滚点：若依赖变更超出预期，恢复本阶段 `go.mod`/`go.sum` 后重新选择明确的 Air 版本，不修改业务代码。

## 7. P2｜Air 配置

### P2-1｜创建 `.air.toml`

新增 `apps/server-go/.air.toml`，实现：

- 根目录为当前服务端项目；
- 默认构建 `./cmd/server`；
- 默认入口为 `bin/fixpro-server-dev`；
- Windows 构建和入口使用 `fixpro-server-dev.exe`；
- 只监听 Go 文件；
- 排除 `bin`、`tmp`、`vendor`、`testdata`；
- 排除 `*_test.go`；
- 防抖约 1000ms；
- 构建失败时不启动旧构建产物；
- Linux/macOS 重启优先发送中断信号；Windows 按 Air 官方行为使用 `TASKKILL`，所有平台配置有限的强制终止延迟；
- 关闭 Air 默认启动横幅。

注意：Air 配置字段必须以实际锁定版本的 `go tool air -h` 和官方示例验证，不能只依据旧版本字段名。优先使用 `build.entrypoint`，不使用已弃用的 `build.bin`。

### P2-2｜平台配置检查

Windows 目标：

```text
cmd: go build -o ./bin/fixpro-server-dev.exe ./cmd/server
entrypoint: bin\\fixpro-server-dev.exe
```

Linux/WSL/macOS 目标：

```text
cmd: go build -o ./bin/fixpro-server-dev ./cmd/server
entrypoint: ./bin/fixpro-server-dev
```

验收：

- TOML 可以被 Air 读取；
- Windows 入口不依赖当前 PATH；
- Linux/WSL 入口不带 `.exe`；
- Air 不监听自己生成的 `bin` 文件。

### P2-3｜构建产物护栏

确认根目录 `.gitignore` 已忽略：

- `apps/server-go/bin/`
- `*.log`
- 本地 `.env`

必要时只补充 Air 产生的开发文件规则，不删除已有忽略规则。

## 8. P3｜开发命令与文档

### P3-1｜Makefile 增加开发命令

在 `apps/server-go/Makefile` 增加：

```make
dev:
	go tool air -c .air.toml
```

保留现有目标：

- `run`：无 Air 的手动启动；
- `migrate`：数据库迁移；
- `build`：普通构建；
- `test`：测试。

### P3-2｜更新根 README

更新“本机启动 Go 后端”章节，明确顺序：

```powershell
cd apps/server-go
$env:DB_DSN='postgres://fixpro:fixpro-local@localhost:5433/fix_pro?sslmode=disable&timezone=UTC'
go run ./cmd/migrate
go tool air -c .air.toml
```

同时说明：

- Air 是本地开发热重载工具；
- 修改 SQL 不会自动执行迁移；
- 不需要全局安装 `air`；
- 生产环境继续使用 Docker/编译产物。

### P3-3｜更新本地开发手册

在 `docs/runbooks/local-development.md` 增加：

- Air 启动步骤；
- PowerShell 环境变量示例；
- `go tool air -d` 调试命令；
- 端口占用排查；
- 构建失败恢复；
- Windows 与 WSL 入口差异；
- 迁移必须单独执行的说明。

验收：

- 新成员只阅读 README 和 runbook 即可启动；
- 命令与实际 Makefile、`.air.toml` 一致；
- 不写入真实密码和密钥。

## 9. P4｜Windows 自动重载验收

### P4-1｜首次启动

准备 PostgreSQL 和环境变量后执行：

```powershell
cd apps/server-go
go run ./cmd/migrate
go tool air -c .air.toml
```

验证：

```powershell
Invoke-WebRequest http://localhost:8080/actuator/health
Invoke-WebRequest http://localhost:8080/api/v1/public/ping
Get-ChildItem .\bin
```

通过条件：

- Air 构建成功；
- `bin/fixpro-server-dev.exe` 存在；
- 健康和 ping 接口返回 200；
- 服务日志显示正常启动；
- 没有第二个服务进程抢占 8080。

### P4-2｜Go 源码自动重载

选择不改变业务结果的 Go 文件，保存一个可识别的非业务修改，例如增加无影响的日志上下文或等价代码调整。记录：

- 修改时间；
- Air 检测时间；
- 构建完成时间；
- 新服务可访问时间。

通过条件：

- Air 检测并重新构建；
- 旧进程被优雅停止；
- 新进程重新监听 8080；
- 5 秒内健康接口恢复；
- `bin` 下没有多份未预期产物。

测试后恢复无影响代码修改，避免将验收痕迹留在业务代码中。

### P4-3｜构建失败恢复

临时制造语法错误，只在验证分支/工作区进行：

1. 保存错误代码；
2. 确认 Air 输出构建失败；
3. 确认不会启动损坏产物；
4. 修复代码；
5. 确认 Air 自动恢复服务。

通过条件：失败不会破坏最后一个可运行进程的可观察行为，修复后自动恢复。

### P4-4｜监听范围验证

分别修改并观察 Air 日志：

- Markdown；
- 图片；
- `*_test.go`；
- `db/migrations/*.sql`；
- `bin/` 中的产物。

通过条件：以上文件不触发主服务重新构建。SQL 迁移仍通过 `go run ./cmd/migrate` 单独执行。

## 10. P5｜Linux/WSL 兼容验收

在 WSL 或 Linux 环境可用时，进入同一服务目录执行：

```bash
go tool air -c .air.toml
```

验证：

- Air 能读取同一份配置；
- 生成 `bin/fixpro-server-dev`；
- 无 `.exe` 入口错误；
- 修改 Go 文件后服务自动重载；
- 健康接口返回 200。

如当前环境没有可用 WSL/Linux，不伪造通过结果；记录为待验收项，不阻塞 Windows 实施，但 PLAN-008 不能宣称跨平台全部完成。

## 11. P6｜生产隔离与质量门禁

### P6-1｜普通 Go 构建

```powershell
cd apps/server-go
go build ./cmd/server ./cmd/migrate
```

通过条件：普通构建不要求 Air 正在运行，不受 `.air.toml` 影响。

### P6-2｜Docker 构建检查

```powershell
docker build -t fixpro-server-go:local .
```

检查：

- Dockerfile 仍直接编译 `cmd/server` 和 `cmd/migrate`；
- 生产镜像入口仍为 `/app/server`；
- 镜像中没有 Air 工具、`.air.toml` 和开发二进制；
- 不把本地数据库凭据写入镜像。

### P6-3｜Go 质量检查

```powershell
gofmt -w .
go vet ./...
go test ./...
```

如果 `gofmt -w .` 触碰了无关预存文件，应停止并只对本次新增/修改的 Go 文件执行格式化；不得顺带重构业务代码。

### P6-4｜原有命令回归

确认以下命令仍然有效：

```powershell
go run ./cmd/server
go run ./cmd/migrate
make run
make build
make test
```

其中 `go run ./cmd/server` 和 `make run` 只在端口空闲且依赖已准备时执行。

## 12. P7｜收尾、回滚与交付

### P7-1｜变更清单

最终允许的代码变更：

- `apps/server-go/go.mod`
- `apps/server-go/go.sum`
- `apps/server-go/.air.toml`
- `apps/server-go/Makefile`
- `README.md`
- `docs/runbooks/local-development.md`
- 必要的 `.gitignore` 规则

### P7-2｜清理本地产物

停止 Air 后确认：

```powershell
git status --short
Get-ChildItem apps/server-go/bin -Force
```

要求：

- Air 生成的二进制保持被忽略；
- 不提交日志、`.env`、临时构建产物；
- 不删除用户已有的未跟踪文件；
- 不删除数据库数据或媒体数据。

### P7-3｜回滚策略

如果 Air 导致本地启动异常：

1. 停止 Air；
2. 使用 `go run ./cmd/server` 恢复开发；
3. 暂时移除 `make dev` 入口或回退 `.air.toml`；
4. 保留 Go 业务代码和数据库不变；
5. 使用 `git diff` 精确回退 Air 相关文件。

生产回滚不涉及 Air，因为生产入口和镜像构建没有改变。

### P7-4｜最终验收追踪

建立验收记录：

| Spec 验收项 | 结果 | 证据 |
| --- | --- | --- |
| AC-01 工具可用 | 待执行 | `go tool air -v` 输出 |
| AC-02 首次启动 | 待执行 | 健康接口响应 |
| AC-03 自动重载 | 待执行 | Air 日志与恢复时间 |
| AC-04 失败恢复 | 待执行 | 构建失败/恢复日志 |
| AC-05 无关文件不重启 | 待执行 | Air 监听日志 |
| AC-06 平台兼容 | 待执行 | Windows、WSL/Linux 记录 |
| AC-07 生产隔离 | 待执行 | Go/Docker 构建结果 |
| AC-08 质量门禁 | 待执行 | test/vet/status 输出 |

## 13. 风险与处理

| 风险 | 处理 |
| --- | --- |
| Air 最新版本字段变化 | 先锁定版本，再用 `go tool air -h` 和官方示例校验配置 |
| Windows exe 被占用 | Windows 使用 Air 的 `TASKKILL` 行为，设置有限 `kill_delay`；确认没有重复服务进程 |
| 修改 SQL 未自动生效 | 明确这是设计行为，单独执行 `go run ./cmd/migrate` |
| Air 监听自身产物 | 排除 `bin`、`tmp`，只监听 Go 文件 |
| 全局 PATH 有旧 Air | 所有文档和 Makefile 使用 `go tool air` |
| 网络无法下载依赖 | 使用团队 Go 代理/模块缓存；不提交二进制替代依赖 |
| 本地已有脏改动 | 实施前记录 `git status`，只修改计划内文件 |

## 14. 推荐提交批次

建议拆为两个提交：

1. `chore(server): add project-local air dev reload`
   - `go.mod`、`go.sum`、`.air.toml`、`Makefile`、必要的 `.gitignore`
2. `docs(server): document air development workflow`
   - `README.md`
   - `docs/runbooks/local-development.md`

若团队希望保持单提交，也必须在提交说明中明确“Air 仅用于本地开发，生产入口未变”。

## 15. 完成定义

以下条件全部满足后，PLAN-008 才算完成：

1. Air 作为项目内工具依赖已锁定；
2. `.air.toml` 能在 Windows 正常启动并重载；
3. Linux/WSL 已验收，或明确记录为环境受限的待验收项；
4. 构建失败后可自动恢复；
5. 无关文件不触发主服务重建；
6. 数据库迁移没有被 Air 隐式执行；
7. 普通 Go 构建、Docker 构建、测试和 vet 通过；
8. 生产镜像和入口不包含 Air；
9. 文档、排障和回滚说明已更新；
10. `git status` 不包含 Air 生成的未忽略产物。
