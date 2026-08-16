# SPEC-006｜师傅作业微信小程序

**版本：** V0.1（工程初始化）  
**日期：** 2026-08-09  
**对应工程：** `apps/wechat-worker-mini`  
**关联后端：** `apps/server-go` 的 Worker API  
**用户角色：** 已认证的维修师傅

> **认证说明：** 本文初始化阶段的 `local-worker-1` 演示 Token 方案已由 `SPEC-010-师傅账号登录、首次改密与作业权限体系` supersede。实际使用必须以手机号登录、首次改密和 Worker session 为准。

## 1. 背景与目标

当前客户微信小程序和管理后台已经具备下单、派单、师傅档案及工种技能配置能力，但师傅没有独立的移动作业入口。新建一个独立微信小程序，承载师傅在手机上的工单查看和后续履约操作，避免与客户端的身份、导航和数据权限混用。

本期工程初始化目标：

- 建立独立的小程序工程、配置、TypeScript 类型检查和开发文档；
- 提供“工作台、我的工单、我的”三 Tab 入口；
- 接入现有 `GET /api/v1/worker/work-orders`，验证师傅 Token、工单列表和空态/错误态；
- 为后续签到、接单、上门、证据上传、完工提交预留页面和服务层扩展位置。

本期不承诺完整履约闭环，初始化页面中的统计和个人资料可以是占位数据，但不得伪装成已完成的真实业务能力。

## 2. 产品范围

### 2.1 本期交付

| 模块 | 本期行为 | 数据来源 |
| --- | --- | --- |
| 工作台 | 展示今日待办四类统计的初始化占位；说明后续作业能力 | 当前本地页面状态 |
| 我的工单 | 使用师傅身份请求工单列表；展示工单号、订单号、状态、预约时间、地址；支持加载、空态、错误态 | `GET /api/v1/worker/work-orders` |
| 我的 | 展示本地演示师傅信息占位 | 当前本地页面状态 |
| 身份令牌 | 开发环境保存 `fixpro.worker.accessToken`；默认使用 `local-worker-1` | 本地配置 |

### 2.2 明确不做

- 师傅微信登录、手机号绑定、实名和账号注册；
- 接单、拒单、签到、到达、服务中、完工提交、返工和证据上传；
- 推送通知、地图导航、电话拨打、离线缓存和消息中心；
- 师傅资料编辑、工种技能编辑和薪资结算；
- 与客户小程序共享登录 Token、Storage Key 或页面入口。

## 3. 信息架构与入口

底部导航固定为：

```text
工作台  |  工单  |  我的
```

页面路径：

| 页面 | 路径 | 说明 |
| --- | --- | --- |
| 工作台 | `pages/workbench/index` | 师傅作业首页 |
| 我的工单 | `pages/work-orders/index` | 当前师傅的工单列表 |
| 我的 | `pages/profile/index` | 师傅身份和账号信息占位 |

该工程必须单独导入微信开发者工具目录 `apps/wechat-worker-mini`，不能通过客户小程序的底部导航进入。

## 4. 身份与数据权限

### 4.1 初始化阶段

- `app.ts` 首次启动写入 `local-worker-1`；
- 请求统一发送 `Authorization: Bearer local-worker-1`；
- 后端 local 环境通过 Worker middleware 解析 Token 中的师傅 ID；
- 生产环境禁止使用本地 Token，必须替换为微信登录态换取的服务端 Worker access token。

### 4.2 权限边界

- 师傅只能读取当前身份被分配的工单；
- 小程序不得调用管理后台 `/api/v1/admin/**` 接口；
- 任意工单详情、状态变更和媒体上传接口都必须由后端再次校验 `assignee_id`；
- 前端隐藏按钮不作为安全措施，越权请求必须由后端返回 403/404。

## 5. 接口契约

### 5.1 已接入接口

`GET /api/v1/worker/work-orders`

请求头：

```http
Authorization: Bearer local-worker-1
```

响应数据最小字段：

```json
{
  "items": [
    {
      "id": "1",
      "workOrderNo": "WO202608090001",
      "orderNo": "FX202608090001",
      "status": "PENDING_ACCEPT",
      "appointmentAt": "2026-08-10T10:00:00Z",
      "serviceAddress": "上海市浦东新区示例路 1 号"
    }
  ],
  "total": 1
}
```

### 5.2 后续接口预留

后续按履约 Spec 接入：

- `POST /api/v1/worker/work-orders/{id}/accept`
- `POST /api/v1/worker/work-orders/{id}/arrive`
- `POST /api/v1/worker/work-orders/{id}/start`
- `POST /api/v1/worker/work-orders/{id}/evidence`
- `POST /api/v1/worker/work-orders/{id}/submit-completion`

具体状态机、幂等键、并发校验和媒体规则以 `SPEC-004` 为准，本 Spec 不重复定义。

## 6. 工程约定

- 工程目录：`apps/wechat-worker-mini`；
- 使用微信原生 TypeScript + WXML + WXSS，不复制客户小程序页面代码；
- API 基础地址集中在 `miniprogram/config/env.ts`；
- 网络请求集中在 `miniprogram/services/request.ts`；
- 工单数据类型和请求集中在 `miniprogram/services/work-orders.ts`；
- 所有页面必须有 loading、空数据和失败状态；
- `npm run mini:worker:typecheck` 必须通过。

## 7. 验收标准

### AC-006-01 工程可导入

微信开发者工具导入 `apps/wechat-worker-mini` 后，能够识别 `miniprogram/app.json`，无“找不到页面文件”错误。

### AC-006-02 导航可用

工作台、工单、我的三个 Tab 均可点击切换，页面标题与当前模块一致。

### AC-006-03 工单接口可用

使用本地后端和 `local-worker-1` Token，工单页能成功请求并展示接口返回的工单；无工单时展示空态。

### AC-006-04 错误可见

后端不可用或 Token 无效时，页面展示可理解的错误提示，不出现未捕获异常白屏。

### AC-006-05 类型检查通过

在仓库根目录执行 `npm run mini:worker:typecheck` 退出码为 0。

### AC-006-06 权限隔离

代码中不存在对管理后台 API 的调用；后续新增作业接口必须通过 Worker middleware 和当前师傅 ID 校验。

## 8. 本地验证步骤

1. 确认 Go 后端运行在 `http://localhost:8080`。
2. 在仓库根目录执行 `npm run mini:worker:typecheck`。
3. 微信开发者工具导入 `D:\work\fix-pro\apps\wechat-worker-mini`。
4. 开发环境使用 `touristappid`，关闭域名校验后编译。
5. 打开“我的工单”，确认请求带有 `Bearer local-worker-1`。
6. 如果数据库中没有分配给师傅 ID 1 的工单，页面应显示“当前没有待处理工单”，这属于正常初始化结果。

## 9. 后续拆分建议

下一阶段优先实现“工单详情 + 接单/拒单”，然后实现“到达/服务中 + 现场证据”，最后实现“完工提交 + 管理后台审核”，每个阶段都补充真实 PostgreSQL 并发、幂等和越权测试。
