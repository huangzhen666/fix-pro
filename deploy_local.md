# 灵狸护家 本地部署与快速启动

## 1. 组件和启动目标

日常启动目标：PostgreSQL 30 秒内就绪，Go 后端 60 秒内启动，React 管理后台 10 秒内启动；任何一步失败都立即输出原因，不无限等待。

| 组件 | 目录/数据 | 地址 |
| --- | --- | --- |
| PostgreSQL 18 | .tmp/postgres-data | 127.0.0.1:5433 |
| Go 后端 + Air | apps/server-go | http://localhost:8080 |
| React 管理后台 + Vite | apps/admin-web | http://localhost:5173 |
| 客户小程序 | apps/wechat-mini | 微信开发者工具 |
| 师傅小程序 | apps/wechat-worker-mini | 微信开发者工具 |

## 2. 一次性初始化

新机器、清理依赖或切换分支后执行：

    go version
    node --version
    npm --version

要求 Go 1.26+、Node.js 24+。

初始化 Go 依赖：

    cd D:\work\fix-pro\apps\server-go
    $env:GOCACHE='D:\work\fix-pro\.gocache'
    $env:GOPATH='D:\work\fix-pro\.gopath'
    $env:GOMODCACHE='D:\work\fix-pro\.gopath\pkg\mod'
    go mod download
    go tool air -v

第一次执行可能需要下载 Air 及其依赖，耗时会明显高于后续启动。后续必须复用 D:\work\fix-pro\.gocache，避免系统缓存目录权限错误。

初始化前端依赖：

    cd D:\work\fix-pro
    npm install

只有 package-lock.json 或工作区依赖发生变化时才需要重新执行。

## 3. 日常快速启动

建议使用三个终端，不要用一个脚本串行等待所有常驻进程。

### 3.0 推荐：一条命令快速启动

从项目根目录执行：

    cd D:\work\fix-pro
    .\scripts\start-local.ps1

这个脚本会：

- 使用 500 毫秒级 TCP 探测，避免端口未监听时 `Test-NetConnection` 阻塞几十秒；
- PostgreSQL、Go/Air、Vite 已经运行时直接复用，不重复启动；
- 默认不重复执行数据库迁移；只有迁移文件或数据库结构发生变化时才执行：

      .\scripts\start-local.ps1 -Migrate

- 在 `.tmp/server-air-local.*.log` 和 `.tmp/admin-web-local.*.log` 保存后台进程日志；
- 启动完成后自动检查本文第 4 节的四个 HTTP 地址。

日常启动不需要执行 `npm install`、`go mod download` 或 `go run ./cmd/migrate`。这些只属于一次性初始化或明确发生依赖/数据库变更的场景。冷启动通常应在 1 分钟内完成；首次下载 Go 工具或恢复 PostgreSQL WAL 时可能更久。

### 终端 A：启动 PostgreSQL

    $pgData='D:\work\fix-pro\.tmp\postgres-data'
    $pg='D:\work\fix-pro\.tmp\postgres-portable\pgsql\bin\postgres.exe'
    $pgOut='D:\work\fix-pro\.tmp\postgres-local.out.log'
    $pgErr='D:\work\fix-pro\.tmp\postgres-local.err.log'
    $ready=Test-NetConnection 127.0.0.1 -Port 5433 -InformationLevel Quiet

    if (-not $ready) {
      $pgPidPath=Join-Path $pgData 'postmaster.pid'
      if (Test-Path $pgPidPath) {
        $oldPidText=(Get-Content $pgPidPath -TotalCount 1).Trim()
        $oldPid=0
        if ([int]::TryParse($oldPidText,[ref]$oldPid)) {
          $oldProcess=Get-Process -Id $oldPid -ErrorAction SilentlyContinue
          if (-not $oldProcess) {
            Move-Item -LiteralPath $pgPidPath -Destination ($pgPidPath+'.stale') -Force
          }
        }
      }
      Start-Process -FilePath $pg -ArgumentList '-D',$pgData,'-p','5433','-h','127.0.0.1' -WorkingDirectory 'D:\work\fix-pro\.tmp\postgres-portable\pgsql\bin' -RedirectStandardOutput $pgOut -RedirectStandardError $pgErr -WindowStyle Hidden
    }

    $deadline=(Get-Date).AddSeconds(30)
    do {
      Start-Sleep -Milliseconds 500
      $ready=Test-NetConnection 127.0.0.1 -Port 5433 -InformationLevel Quiet
    } while (-not $ready -and (Get-Date) -lt $deadline)

    if (-not $ready) {
      Get-Content $pgErr -Tail 50 -ErrorAction SilentlyContinue
      throw 'PostgreSQL 未能在 30 秒内监听 5433'
    }
    Write-Host 'PostgreSQL ready: 127.0.0.1:5433'

不要删除 .tmp/postgres-data；其中保存本地业务数据。旧 postmaster.pid 只有在确认对应进程不存在时才会改名备份。

### 终端 B：迁移并启动 Go 后端

数据库刚重建或迁移文件发生变化时执行一次：

    cd D:\work\fix-pro\apps\server-go
    $env:GOCACHE='D:\work\fix-pro\.gocache'
    $env:GOPATH='D:\work\fix-pro\.gopath'
    $env:GOMODCACHE='D:\work\fix-pro\.gopath\pkg\mod'
    $env:DB_DSN='postgres://fixpro:fixpro-local@localhost:5433/fix_pro?sslmode=disable&timezone=UTC'
    go run ./cmd/migrate

师傅登录体系首次上线或存在历史 `!local-worker-*` 占位密码时，再执行一次存量账号转换：

    go run ./cmd/backfill-worker-auth

该命令只转换历史占位密码，生成 Argon2id(`w+手机号`) 哈希，并标记师傅首次登录必须改密；已经改过正式密码的账号不会被覆盖。新建师傅由后台接口直接生成初始密码。

首次启用后台权限体系时初始化管理员（默认 `admin / 12345678`，密码不输出到日志；已有同名用户会被重置为初始化密码并要求改密）：

    go run ./cmd/bootstrap-admin -org-id 1 -username admin -display-name 本地管理员 -platform-super-admin

然后启动 Air：

    $env:APP_ENV='local'
    $env:HTTP_ADDR=':8080'
    $env:MEDIA_DRIVER='local'
    $env:MEDIA_LOCAL_ROOT='D:\work\fix-pro\.tmp\media-current'
    $env:APP_ADMIN_USERNAME='admin'
    $env:APP_ADMIN_PASSWORD='12345678'
    $env:APP_ADMIN_BASIC_COMPAT='false' # 默认关闭 Basic Auth；仅应急回滚时临时设为 true
    $env:APP_ADMIN_COOKIE_SECURE='false' # 本地 HTTP 使用 false，HTTPS 生产必须 true
    $env:WORKER_DEV_TOKEN_ENABLED='false' # 默认使用手机号登录；仅临时联调才显式开启
    $env:CORS_ALLOWED_ORIGINS='http://localhost:5173'
    go tool air -c .air.toml

日常只修改 Go 源码时，不需要重复执行迁移，直接启动 Air 即可。

### 终端 C：启动 React 管理后台

    cd D:\work\fix-pro\apps\admin-web
    npm run dev -- --host 127.0.0.1

Vite 会将 /api 代理到 http://127.0.0.1:8080。

## 4. 启动完成检查

    Invoke-WebRequest http://127.0.0.1:8080/actuator/health -UseBasicParsing
    Invoke-WebRequest http://127.0.0.1:8080/api/v1/public/ping -UseBasicParsing
    Invoke-WebRequest http://127.0.0.1:5173/ -UseBasicParsing
    Invoke-WebRequest http://127.0.0.1:5173/api/v1/public/ping -UseBasicParsing

四个请求均返回 HTTP 200，才认为本地项目启动完成。

师傅认证链路检查：

1. 后台新增并启用师傅，记录一次性初始密码 `w+手机号`；
2. 师傅小程序导入 `apps/wechat-worker-mini`，使用手机号和初始密码登录；
3. 确认工单接口返回 423，完成首次改密后自动回到登录页；
4. 使用新密码登录并确认只能看到本人被派发的工单；
5. 后台执行“重置密码”后，确认旧密码和旧 Token 失效。

## 5. 端口和进程检查

    Get-NetTCPConnection -State Listen |
      Where-Object LocalPort -in 5433,8080,5173 |
      Select-Object LocalAddress,LocalPort,OwningProcess

预期：5433 为 PostgreSQL，8080 为 Air/Go，5173 为 Vite。不要使用 Stop-Process -Name go 或 Stop-Process -Name node，避免误杀其他开发工具；应根据端口查到 PID 后只停止对应进程。

## 6. 常见问题

### 6.1 启动超过几分钟

    go version
    go tool air -v
    Test-NetConnection 127.0.0.1 -Port 5433
    Test-NetConnection 127.0.0.1 -Port 8080
    Test-NetConnection 127.0.0.1 -Port 5173

处理原则：

- go tool air -v 很慢：通常是第一次下载工具依赖，确认使用 D:\work\fix-pro\.gocache；
- 5433 不通：检查 PostgreSQL 日志，不要继续等待后端；
- 8080 不通：读取 Air 日志，通常是编译失败或数据库连接失败；
- 5173 不通：确认从 apps/admin-web 启动；
- Docker daemon 不可用：不要等待 Compose，直接使用本地 PostgreSQL 方案。

如果师傅小程序提示使用演示身份，检查 `app.ts` 和 `WORKER_DEV_TOKEN_ENABLED`；生产和日常本地验证均不应自动写入 `local-worker-1`。

### 6.2 Go 构建缓存权限错误

出现 Access is denied 时，先执行：

    $env:GOCACHE='D:\work\fix-pro\.gocache'
    $env:GOPATH='D:\work\fix-pro\.gopath'
    $env:GOMODCACHE='D:\work\fix-pro\.gopath\pkg\mod'

再执行 go tool air、go test 或 go build。

### 6.3 PostgreSQL 连接失败

    & 'D:\work\fix-pro\.tmp\postgres-portable\pgsql\bin\psql.exe' -h 127.0.0.1 -p 5433 -U fixpro -d fix_pro -w -c 'select current_database(), current_user'

如果失败：

1. 检查 5433 是否监听；
2. 查看 .tmp/postgres-local.err.log；
3. 确认没有其他 PostgreSQL 实例占用 5433；
4. 不要把项目 DSN 改成 Windows PostgreSQL 服务的 5432，除非已明确配置对应用户、数据库和密码。

### 6.4 Air 构建失败

    cd D:\work\fix-pro\apps\server-go
    $env:GOCACHE='D:\work\fix-pro\.gocache'
    $env:GOPATH='D:\work\fix-pro\.gopath'
    $env:GOMODCACHE='D:\work\fix-pro\.gopath\pkg\mod'
    go build ./cmd/server
    go tool air -c .air.toml -d

修改 SQL 不会自动迁移，必须单独执行 go run ./cmd/migrate。

## 7. 停止本地项目

Air 和 Vite 在各自终端按 Ctrl+C。

停止 PostgreSQL：

    & 'D:\work\fix-pro\.tmp\postgres-portable\pgsql\bin\pg_ctl.exe' -D 'D:\work\fix-pro\.tmp\postgres-data' -m fast stop

停止后再次检查 5433、8080、5173 监听状态。

## 8. Docker Compose 方案

Docker Desktop 正常运行时：

    docker compose -f deploy/compose.yaml up -d --build postgres migrate server

Docker 不可用时不要反复重试，切换到本文第 3 节的本地 PostgreSQL + Air 方案。

## 9. 微信小程序

命令行只负责后端和管理后台。使用微信开发者工具导入：

    D:\work\fix-pro\apps\wechat-mini
    D:\work\fix-pro\apps\wechat-worker-mini

本地接口地址为 http://localhost:8080。开发者工具中需要关闭“校验合法域名、业务域名、TLS 版本以及 HTTPS 证书”。

## 10. 日志位置

| 日志 | 用途 |
| --- | --- |
| .tmp/postgres-local.err.log | PostgreSQL 启动和恢复 |
| .tmp/server-air-20260814.err.log | Air 监听、构建、启动 |
| .tmp/admin-web-20260814.out.log | Vite 启动日志 |
| Air 配置 | apps/server-go/.air.toml |

日志只用于本地排障，不要提交到 Git。

## 11. 快速启动完成定义

以下四个请求均返回 200：

    Invoke-WebRequest http://127.0.0.1:8080/actuator/health -UseBasicParsing
    Invoke-WebRequest http://127.0.0.1:8080/api/v1/public/ping -UseBasicParsing
    Invoke-WebRequest http://127.0.0.1:5173/ -UseBasicParsing
    Invoke-WebRequest http://127.0.0.1:5173/api/v1/public/ping -UseBasicParsing

不要使用无限等待的启动脚本；每个依赖都必须先通过端口检查，再进入下一步。

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
