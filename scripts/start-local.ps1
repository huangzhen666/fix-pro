param(
  [switch]$Migrate
)

$ErrorActionPreference = 'Stop'

$ProjectRoot = 'D:\work\fix-pro'
$ServerDir = Join-Path $ProjectRoot 'apps\server-go'
$AdminDir = Join-Path $ProjectRoot 'apps\admin-web'
$TmpDir = Join-Path $ProjectRoot '.tmp'
$PgData = Join-Path $TmpDir 'postgres-data'
$PgBinDir = Join-Path $TmpDir 'postgres-portable\pgsql\bin'
$Pg = Join-Path $PgBinDir 'postgres.exe'
$SystemPg = 'C:\Program Files\PostgreSQL\18\bin\postgres.exe'
$PgErr = Join-Path $TmpDir 'postgres-local.err.log'
$PgOut = Join-Path $TmpDir 'postgres-local.out.log'
$MediaRoot = Join-Path $TmpDir 'media-current'

New-Item -ItemType Directory -Force -Path $TmpDir, $MediaRoot | Out-Null

function Test-LocalPort {
  param(
    [Parameter(Mandatory = $true)][int]$Port,
    [int]$TimeoutMs = 500
  )

  $client = [System.Net.Sockets.TcpClient]::new()
  try {
    $async = $client.BeginConnect('127.0.0.1', $Port, $null, $null)
    if (-not $async.AsyncWaitHandle.WaitOne($TimeoutMs)) {
      return $false
    }
    $client.EndConnect($async)
    return $true
  } catch {
    return $false
  } finally {
    $client.Dispose()
  }
}

function Wait-LocalPort {
  param(
    [Parameter(Mandatory = $true)][int]$Port,
    [int]$TimeoutSeconds = 30
  )

  $deadline = [DateTime]::UtcNow.AddSeconds($TimeoutSeconds)
  while ([DateTime]::UtcNow -lt $deadline) {
    if (Test-LocalPort -Port $Port) {
      return
    }
    Start-Sleep -Milliseconds 200
  }
  throw "端口 $Port 未能在 $TimeoutSeconds 秒内就绪"
}

function Wait-HttpOk {
  param(
    [Parameter(Mandatory = $true)][string]$Url,
    [int]$TimeoutSeconds = 30
  )

  $deadline = [DateTime]::UtcNow.AddSeconds($TimeoutSeconds)
  while ([DateTime]::UtcNow -lt $deadline) {
    try {
      $response = Invoke-WebRequest -Uri $Url -UseBasicParsing -SkipHttpErrorCheck -TimeoutSec 3
      if ([int]$response.StatusCode -eq 200) {
        return
      }
    } catch {
      # 服务启动过程中连接失败，继续短暂重试；超过总超时才报错。
    }
    Start-Sleep -Milliseconds 300
  }
  throw "$Url 未能在 $TimeoutSeconds 秒内返回 HTTP 200"
}

function Invoke-GoCommand {
  param([Parameter(Mandatory = $true)][string[]]$Arguments)

  Push-Location $ServerDir
  try {
    & $script:Go @Arguments
    if ($LASTEXITCODE -ne 0) {
      throw "Go 命令失败: go $($Arguments -join ' ')"
    }
  } finally {
    Pop-Location
  }
}

$script:Go = (Get-Command go.exe -ErrorAction Stop).Source
$script:Npm = (Get-Command npm.cmd -ErrorAction Stop).Source
$env:GOCACHE = Join-Path $ProjectRoot '.gocache'
$env:GOPATH = Join-Path $ProjectRoot '.gopath'
$env:GOMODCACHE = Join-Path $env:GOPATH 'pkg\mod'
New-Item -ItemType Directory -Force -Path $env:GOPATH, $env:GOMODCACHE | Out-Null
$env:DB_DSN = 'postgres://fixpro:fixpro-local@localhost:5433/fix_pro?sslmode=disable&timezone=UTC'
$env:APP_ENV = 'local'
$env:HTTP_ADDR = ':8080'
$env:MEDIA_DRIVER = 'local'
$env:MEDIA_LOCAL_ROOT = $MediaRoot
$env:APP_ADMIN_USERNAME = 'admin'
$env:APP_ADMIN_PASSWORD = 'change-me-in-production'
$env:APP_ADMIN_BASIC_COMPAT = 'false'
$env:APP_ADMIN_COOKIE_SECURE = 'false'
$env:WORKER_DEV_TOKEN_ENABLED = 'false'
$env:CORS_ALLOWED_ORIGINS = 'http://localhost:5173'

if (-not (Test-LocalPort -Port 5433)) {
  $postgresExecutable = $Pg
  if (-not (Test-Path $postgresExecutable)) {
    $postgresExecutable = $SystemPg
  }
  if (-not (Test-Path $postgresExecutable)) {
    throw "找不到 PostgreSQL: $Pg 或 $SystemPg"
  }
  $pidPath = Join-Path $PgData 'postmaster.pid'
  if (Test-Path $pidPath) {
    $oldPidText = (Get-Content $pidPath -TotalCount 1).Trim()
    $oldPid = 0
    if ([int]::TryParse($oldPidText, [ref]$oldPid) -and -not (Get-Process -Id $oldPid -ErrorAction SilentlyContinue)) {
      Move-Item -LiteralPath $pidPath -Destination ($pidPath + '.stale') -Force
    }
  }
  Start-Process -FilePath $postgresExecutable -ArgumentList '-D', $PgData, '-p', '5433', '-h', '127.0.0.1' -WorkingDirectory (Split-Path -Parent $postgresExecutable) -RedirectStandardOutput $PgOut -RedirectStandardError $PgErr -WindowStyle Hidden | Out-Null
  Wait-LocalPort -Port 5433 -TimeoutSeconds 30
  Write-Host 'PostgreSQL ready: 127.0.0.1:5433'
} else {
  Write-Host 'PostgreSQL already running: 127.0.0.1:5433'
}

if ($Migrate) {
  Write-Host 'Applying database migrations...'
  Invoke-GoCommand -Arguments @('run', './cmd/migrate')
}

if (-not (Test-LocalPort -Port 8080)) {
  $serverOut = Join-Path $TmpDir 'server-air-local.out.log'
  $serverErr = Join-Path $TmpDir 'server-air-local.err.log'
  Start-Process -FilePath $script:Go -ArgumentList 'tool', 'air', '-c', '.air.toml' -WorkingDirectory $ServerDir -RedirectStandardOutput $serverOut -RedirectStandardError $serverErr -WindowStyle Hidden | Out-Null
  Wait-LocalPort -Port 8080 -TimeoutSeconds 60
  Write-Host 'Go + Air ready: http://127.0.0.1:8080'
} else {
  Write-Host 'Go backend already running: http://127.0.0.1:8080'
}

if (-not (Test-LocalPort -Port 5173)) {
  $adminOut = Join-Path $TmpDir 'admin-web-local.out.log'
  $adminErr = Join-Path $TmpDir 'admin-web-local.err.log'
  Start-Process -FilePath $script:Npm -ArgumentList 'run', 'dev', '--', '--host', '127.0.0.1' -WorkingDirectory $AdminDir -RedirectStandardOutput $adminOut -RedirectStandardError $adminErr -WindowStyle Hidden | Out-Null
  Wait-LocalPort -Port 5173 -TimeoutSeconds 30
  Write-Host 'React admin ready: http://127.0.0.1:5173'
} else {
  Write-Host 'React admin already running: http://127.0.0.1:5173'
}

Wait-HttpOk -Url 'http://127.0.0.1:8080/actuator/health' -TimeoutSeconds 30
Wait-HttpOk -Url 'http://127.0.0.1:8080/api/v1/public/ping' -TimeoutSeconds 30
Wait-HttpOk -Url 'http://127.0.0.1:5173/' -TimeoutSeconds 30
Wait-HttpOk -Url 'http://127.0.0.1:5173/api/v1/public/ping' -TimeoutSeconds 30

Write-Host 'FixPro local startup complete.'
Write-Host 'Use -Migrate only after migration files or database changes; do not run npm install or go mod download for daily startup.'
