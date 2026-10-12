#!/usr/bin/env bash
set -euo pipefail
project_root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$project_root"
mkdir -p .tmp/media-current
export APP_ENV=local DB_DSN='postgres://fixpro:fixpro-local@127.0.0.1:5433/fix_pro?sslmode=disable&timezone=UTC'
export GOCACHE="$project_root/.gocache" GOPATH="$project_root/.gopath" GOMODCACHE="$project_root/.gopath/pkg/mod"
export MEDIA_LOCAL_ROOT="$project_root/.tmp/media-current" CORS_ALLOWED_ORIGINS='http://localhost:5173'
if [[ -f .tmp/customer-auth.env ]]; then
  while IFS= read -r customer_line; do
    customer_key=${customer_line%%=*}; customer_value=${customer_line#*=}
    case "$customer_key" in WECHAT_MINI_APP_ID|WECHAT_MINI_APP_SECRET|CUSTOMER_MOBILE_ENCRYPTION_KEY|CUSTOMER_MOBILE_HASH_KEY|CUSTOMER_DEFAULT_ORG_ID|CUSTOMER_SESSION_TTL) export "$customer_key=$customer_value";; esac
  done < .tmp/customer-auth.env
fi
if ! nc -z 127.0.0.1 5433; then
  pg_ctl -D "$project_root/.tmp/postgres-data" -l "$project_root/.tmp/postgres-local.out.log" -o '-p 5433 -h 127.0.0.1' start
fi
if [[ "${1:-}" == '--migrate' ]]; then go -C apps/server-go run ./cmd/migrate; fi
if ! nc -z 127.0.0.1 8080; then
  go -C apps/server-go build -o "$project_root/.tmp/fixpro-server-local" ./cmd/server
  python3 - <<'PYLAUNCH'
from pathlib import Path
import subprocess
root = Path.cwd()
with open(root/'.tmp/server-local.out.log','a') as out, open(root/'.tmp/server-local.err.log','a') as err:
    p = subprocess.Popen([str(root/'.tmp/fixpro-server-local')], cwd=root/'apps/server-go', stdout=out, stderr=err, stdin=subprocess.DEVNULL, start_new_session=True)
(root/'.tmp/server-local.pid').write_text(str(p.pid))
PYLAUNCH
fi
if ! nc -z 127.0.0.1 5173; then
  python3 - <<'PYLAUNCH'
from pathlib import Path
import subprocess
root = Path.cwd()
with open(root/'.tmp/admin-web-local.out.log','a') as out, open(root/'.tmp/admin-web-local.err.log','a') as err:
    p = subprocess.Popen(['npm','run','admin:dev','--','--host','127.0.0.1','--port','5173','--strictPort'], cwd=root, stdout=out, stderr=err, stdin=subprocess.DEVNULL, start_new_session=True)
(root/'.tmp/admin-web-local.pid').write_text(str(p.pid))
PYLAUNCH
fi
printf 'API http://localhost:8080\n后台 http://localhost:5173\n'
