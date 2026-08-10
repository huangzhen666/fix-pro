# PLAN-004 F6 验收记录

日期：2026-08-09  
环境：Windows、本地 PostgreSQL `localhost:5433`、Go API `localhost:8080`  
数据库：`fix_pro`，migration 已执行至 V2

## 自动化验收

执行命令：

```powershell
$env:FIXPRO_INTEGRATION='1'
$env:DB_DSN='postgres://fixpro:fixpro-local@localhost:5433/fix_pro?sslmode=disable&timezone=UTC'
go test ./test/integration -count=1 -v
```

结果：通过。

覆盖内容：

- PostgreSQL migration 版本大于等于 2；
- 两个并发更新同一工单版本时仅一个成功；
- 客户 A 读取客户 B 订单时返回 404，不泄露资源存在性；
- 测试数据使用唯一订单号并在测试结束后清理。

Go 全量测试：

```powershell
go test ./...
```

结果：通过。

小程序类型检查：

```powershell
npm run mini:typecheck
```

结果：通过。

API 健康检查：

```powershell
Invoke-RestMethod http://localhost:8080/api/v1/public/ping
```

结果：返回 `code=OK`、`data.message=pong`。

## 三端手工 E2E 清单

以下步骤需要使用微信开发者工具和浏览器完成，数据库自动化测试不能替代这些 UI 验收：

1. 小程序客户在“全部服务”选择两个已发布 SKU，分别填写故障描述和故障媒体并加入购物车。
2. 小程序提交订单，确认订单状态为 `PENDING_CONFIRMATION`。
3. React 管理后台打开订单，点击“确认并生成工单”，确认生成两张 `PENDING_DISPATCH` 工单。
4. 后台创建或选择两个启用师傅，分别设置未来预约时间并派单。
5. 师傅 A 使用 `Bearer local-worker-{id}` 接单、到达、开始服务。
6. 师傅 A 上传施工前图片：

   `POST /api/v1/worker/work-orders/{id}/media/images`

7. 师傅 A 使用返回的 `mediaId` 绑定 `BEFORE`，再上传并绑定 `AFTER`。
8. 师傅 A 提交完工，工单进入 `WAITING_COMPLETION_REVIEW`。
9. 后台审核一张工单通过、另一张驳回；驳回工单补证据后再次提交并通过。
10. 客户打开“我的订单”详情，确认工单进度和施工图片可见。
11. 客户逐张确认验收；第一张完成后订单不能提前完成，最后一张完成后订单变为 `COMPLETED`。
12. 后台刷新订单详情，核对订单历史、工单历史、派单历史、完工审核和证据记录。

## 当前未自动化的项目

- 派单、接单、改派、完工审核和客户验收的 HTTP 并发测试尚未加入自动化套件；
- `Idempotency-Key` 在客户验收接口已由小程序发送，但服务层目前主要依靠版本号和状态条件防止重复写入，完整幂等响应重放仍需单独补齐；
- 三端 E2E 需要真实图片文件和微信开发者工具，目前记录为手工验收清单，不能标记为全自动通过。
