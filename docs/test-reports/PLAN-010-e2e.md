# PLAN-010 本地 E2E 验证记录

**日期：** 2026-08-16  
**环境：** Windows、本地 PostgreSQL `127.0.0.1:5433`、Go 临时验证端口 `8081`  
**数据范围：** 本地组织 1，测试师傅账号和既有工单

## 1. 已验证链路

| 场景 | 结果 |
| --- | --- |
| `000013_worker_auth` migration | 通过，`go run ./cmd/migrate` 成功 |
| 存量占位密码回填 | 通过，`go run ./cmd/backfill-worker-auth` 将历史占位账号转为 Argon2id；重复执行无重复更新 |
| 后台新增师傅 | 通过，返回一次性初始密码字段并要求首次改密 |
| 后台启用师傅 | 通过，配置工种和技能后状态变为 ACTIVE |
| 手机号 + 初始密码登录 | 通过，返回独立 Worker Bearer Token 和 `mustChangePassword=true` |
| 首次改密前访问工单 | 通过，返回 `423 WORKER_PASSWORD_CHANGE_REQUIRED` |
| 首次修改密码 | 通过，成功后旧 Token 失效 |
| 新密码重新登录 | 通过，`mustChangePassword=false` |
| 工单列表 | 通过，只返回当前师傅被分配的工单 |
| 访问其他师傅工单详情 | 通过，返回 `WORK_ORDER_NOT_ASSIGNED_TO_YOU` |
| 后台重置密码 | 通过，需要管理员权限，返回一次性临时密码并标记再次改密 |
| 重置后旧密码登录 | 通过，返回 `WORKER_LOGIN_FAILED` |
| 重置后旧 Token | 通过，返回 `WORKER_SESSION_INVALID` |
| `local-worker-*` 演示 Token | 通过，默认配置下返回 `WORKER_SESSION_INVALID` |

## 2. 自动化检查

```text
go test ./...                          PASS
go vet ./...                           PASS
npm run typecheck (wechat-worker-mini) PASS
npm run lint (admin-web)               PASS
npx vite build --configLoader runner   PASS
```

管理后台默认 `npm run build` 会尝试向受限的 `node_modules/.tmp` 写入 tsbuildinfo；本次使用工作区临时 tsbuildinfo 路径完成 TypeScript 检查，并使用 Vite runner 完成生产构建。

## 3. 未完成验证

- 当前环境没有 gcc，`go test -race` 无法启动 CGO 编译；
- 尚未在微信开发者工具中完成登录页和首次改密页的人工点击验收；
- 尚未在真实生产配置和多实例环境执行发布演练。

