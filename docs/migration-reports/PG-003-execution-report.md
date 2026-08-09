# PG-003 PostgreSQL 迁移执行报告

**执行日期：** 2026-08-07  
**结果：** 通过  
**目标库：** PostgreSQL 18.4（本地候选库 `127.0.0.1:5433/fix_pro`）

## 1. 数据决策

- 现有 MySQL 数据需要保留并已完成复制。
- 本次未删除 MySQL 数据目录，旧库不再作为项目运行依赖。
- PostgreSQL 产生新写入后，不能未经数据回迁直接恢复 MySQL 写入。

## 2. 基线与复制

复制前 MySQL 共 19 张业务表，关键记录为：

| 数据 | MySQL 源库 | PostgreSQL 复制后 |
| --- | ---: | ---: |
| organization | 1 | 1 |
| customer | 1 | 1 |
| service_category | 6 | 6 |
| service_sku | 1 | 1 |
| service_sku_version | 1 | 1 |
| media_asset | 1 | 1 |
| customer_order | 0 | 0 |

所有 19 张表均已执行固定列复制，identity 序列已按目标表最大 ID 校准。SKU `created_at` 在 MySQL 和 PostgreSQL UTC 抽样值均为 `2026-08-05 23:07:48.732`。

## 3. Schema 与运行时验证

- PostgreSQL 干净 baseline migration 成功。
- 独立空库 `fix_pro_verify` 使用正式 `db/migrations` 执行成功。
- 业务表 19 张，`schema_migrations` 版本为 `1` 且 `dirty=false`。
- Go 运行时仅使用 pgx，SQL 占位符、upsert、identity、联表删除、唯一冲突和事务幂等语义均已切换。

## 4. 正向链路验收

1. 公开目录返回已迁移并发布的 SKU 及封面图。
2. 管理端新建分类成功，新 ID 为 `1007`，证明序列校准有效。
3. 管理端上传 SKU 图片，新建并发布 SKU `PG-E2E-001`（ID `2`，`8800` 分）。
4. 小程序公开检索立即返回该 SKU 及封面图，客户加购后购物车价格为 `8800` 分。
5. 上传故障图片、填写故障描述后，`faultComplete=true`。
6. 提交订单 `FP20260807125302545600` 成功，管理端订单详情可查到 SKU 快照、客户资料、故障描述和图片。
7. 同一幂等键串行重放返回同一订单。
8. 两个并发请求使用同一幂等键，两个响应均返回订单 `FP20260807125018351900`，数据库只生成 1 条订单。
9. 联调后源 MySQL 仍为 6 个分类、0 个订单，证明测试新写入只进入 PostgreSQL。

## 5. 质量检查

- `go fmt ./...`：通过。
- `go vet ./...`：通过。
- `go test ./...`：通过。
- `go build ./cmd/server ./cmd/migrate`：通过。
- `npm run check`：React lint/build 和小程序 TypeScript 检查通过。
- `docker compose -f deploy/compose.yaml config --quiet`：通过。
- MySQL 运行依赖静态扫描：通过。

## 6. 本地候选库状态

- PostgreSQL 候选库保留在 `.tmp/postgres-data`，端口为 `5433`。
- Go API 已使用该 PostgreSQL 在 `8080` 端口运行。
- MySQL 源进程已停止，源数据保留在 `.tmp/mysql-data`，未删除。
- 正式本地入口已改为 Compose PostgreSQL `5432`。
