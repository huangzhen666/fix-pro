# SPEC-007｜完工并行验收、双级审核与售后争议

**状态：** Draft  
**版本：** V1.3  
**日期：** 2026-08-10  
**适用工程：** `apps/server-go`、`apps/admin-web`、`apps/wechat-mini`、`apps/wechat-worker-mini`  
**关联文档：** `SPEC-004-订单履约与工单派单链路.md`、`SPEC-006-师傅作业微信小程序.md`

> 本 Spec 替代 SPEC-004 中“单级后台完工审核 → 客户验收”的规则；未被本 Spec 修改的派单、履约和证据规则继续有效。

## 1. 背景与问题

当前流程为：

```text
师傅提交完工 → 审核员审核 → 客户验收
```

该流程存在四个问题：

1. 审核员和客户的审核职责没有明确分开，页面和状态容易混为一谈。
2. 单个审核员拥有最终放行权，存在师傅与审核员串通、证据不完整仍放行的风险。
3. 客户拒绝验收、要求返修、发起投诉属于不同业务，当前都压缩为一个“驳回”动作，后续无法统计和追责。
4. 已完成工单发生质保投诉时，如果直接把原工单退回服务中，会破坏原始履约记录、结算和质保起算时间。

合伙人输入明确要求：施工前后影像强制存档、客户线上确认验收、统一质保和售后纠纷处理。本 Spec 在此基础上增加审核员初审和总监复核两级内部审核。

## 2. 核心结论

完整流程采用“双分支并行 + 结果汇合 + 独立售后案件”。客户验收是服务结果的主判断，内部两级审核是平台质量与合规判断，两者互不阻塞发起：

```text
师傅提交完工
├→ 客户立即验收和评价
└→ 审核员初审 → 总监复核
                 ↓
              结果汇合

客户通过 + 内部通过 → 正常结束
客户驳回（无论内部结果）→ 立即进入二次上门
客户通过 + 内部驳回 → 客服联系客户确认是否二次上门

客户拒绝验收或后续投诉
→ 售后案件
→ 受理判责
→ 原师傅返修 / 改派返修 / 退款补偿 / 驳回投诉
→ 客户确认售后结果
```

三类动作职责不同：

| 动作 | 执行人 | 判断内容 | 是否能代替其他环节 |
| --- | --- | --- | --- |
| 初审 | 审核员 | 施工资料、前后对比、完工说明、SOP 是否齐全，并感知客户验收和评价 | 不能覆盖客户决定 |
| 复核 | 总监 | 初审结论、客户反馈、关键证据、异常风险 | 不能覆盖客户决定 |
| 验收 | 客户 | 实际服务是否完成、现场结果是否接受 | 不能代替内部质量审核 |
| 投诉处理 | 客服/售后主管 | 事实调查、责任和解决方案 | 不直接修改历史审核记录 |

## 3. 目标与成功标准

### 3.1 目标

- 师傅提交完工后，客户验收/评价与内部初审同时开放，不互相等待。
- 内部审核仍保持“审核员初审 → 总监复核”的顺序。
- 审核员与总监必须是不同账号，且都不能是执行师傅。
- 每次师傅重新提交形成独立的完工提交批次，旧证据和旧审核记录不可覆盖。
- 客户拒绝验收自动建立售后案件，并进入返修/争议处理流程。
- 已完成工单发生投诉时保留原工单，必要时创建关联返修工单。
- 全过程可查询、可审计、可统计，不允许后台静默修改结论。

### 3.2 成功标准

1. 客户在师傅提交完工后立即看到验收和评价入口，无需等待内部审核。
2. 未完成初审的提交不能进入总监复核，但不影响客户验收。
3. 审核员和总监的页面必须展示客户是否通过、评分、标签和评价内容。
4. 审核员不能执行同一提交批次的总监复核，执行师傅不能参与审核。
5. 客户拒绝验收时原因必填，并立即进入二次上门，不等待内部审核结束。
6. 客户已通过但内部终审驳回时，进入客服确认状态，由客服记录客户是否要求二次上门。
7. 客户确认不需要二次上门时工单正常完结，但内部“不合格”结论仍影响师傅质量记录和结算规则。
8. 已完成工单投诉不会重写原工单的完成状态和历史。
9. 每次二次上门都有独立作业轮次、证据和审核记录。
10. 同一命令重复提交不会生成重复验收、评价、审核或状态历史。
11. 客户确认无需二次上门的工单具有独立结案标记，可筛选并查看完整流程时间线。

## 4. 角色与权限

| 角色 | 代码 | 可执行动作 | 禁止动作 |
| --- | --- | --- | --- |
| 师傅 | `WORKER` | 履约、上传证据、提交完工、处理返修 | 审核自己的工单、关闭投诉 |
| 审核员 | `QA_REVIEWER` | 完工初审、填写问题项、给出通过/驳回建议 | 总监复核、审核本人作为师傅执行的工单 |
| 总监 | `QA_DIRECTOR` | 查看初审和客户反馈、给出内部终审结论 | 跳过初审直接终审；无原因推翻初审结论 |
| 客户 | `CUSTOMER` | 验收、拒绝验收、投诉、补充证据、确认售后结果 | 查看内部备注、指定审核结果 |
| 客服 | `AFTER_SALES_AGENT` | 受理投诉、沟通、补充资料、提出方案 | 单独批准高额退款、删除证据 |
| 售后主管 | `AFTER_SALES_MANAGER` | 判责、批准返修/改派/补偿、关闭案件 | 修改原始完工提交和审核结论 |
| 调度员 | `DISPATCHER` | 派单、改派、安排返修 | 执行质量初审或总监复核 |
| 系统管理员 | `ADMIN` | 账号、角色和组织配置 | 默认不等于审核员或总监；业务审核需显式角色 |

### 4.1 职责隔离规则

- 同一完工提交的 `QA_REVIEWER` 与 `QA_DIRECTOR` 必须是不同 `employee_account.id`。
- 执行师傅不能参与该工单任何级别的内部审核。
- 初审员不能修改自己已提交的审核结果，只能由上级追加纠正记录。
- 总监复核页面必须展示执行师傅、初审员、初审时间和初审结论。
- 初审任务由队列分配或领取，总监任务从独立角色池分配；不允许师傅指定审核人员。
- 组织可配置高风险规则：高金额、投诉返修、多次驳回工单必须由指定总监池处理。

## 5. 主流程

### 5.1 提交后立即分叉

师傅提交完工时，系统原子创建不可修改的完工提交批次和两个并行任务：

```text
COMPLETION_SUBMITTED
├─ 客户分支：PENDING_ACCEPTANCE（立即可验收、评价）
└─ 内审分支：PENDING_QA → PENDING_DIRECTOR → APPROVED / REJECTED
```

客户分支与内部审核分支分别保存状态，不能再用一个 `work_order.status` 同时表达两条进度。工单展示状态由两个分支聚合产生。

### 5.2 客户验收与评价

客户在师傅提交后立即看到：

- 服务项目和完工说明；
- 客户可见的施工前后证据；
- 服务师傅、预约和完工时间；
- 质保范围、期限和除外项；
- “确认完成”“拒绝验收”和评价入口。

客户验收与评价分开记录：

- 验收决定为 `ACCEPT` 或 `REJECT`，每个提交批次只能有一个生效决定；
- 评价包含 1—5 星、标签和文本，不能决定内部审核结论；
- 客户可以先验收后评价，评价不阻塞后续流程；
- 客户评价一旦提交不可修改、不可覆盖；页面提交前必须明确提示并二次确认；
- 审核员和总监打开审核时必须看到当时最新的验收决定和评价；
- 客户尚未操作时明确显示“客户待验收”，不能显示为默认通过。

客户点击通过后立即记录 `customer_completed_at` 并启动质保，不必等待内部审核；前台显示“客户已验收，平台质检中”。最终关单和师傅结算仍按结果汇合规则执行。

### 5.3 内部两级审核

内部审核仍串行执行：

```text
PENDING_QA
→ 审核员给出初审结论
PENDING_DIRECTOR
→ 总监结合初审结论和客户反馈给出内部终审结论
APPROVED / REJECTED
```

审核员初审必须检查：施工前后证据、必要测试证据、完工说明、SKU/SOP 检查项、图片清晰度和异常风险。总监必须看到执行师傅、初审员、初审结论以及客户验收、评分和文字评价。

初审结论无论通过或驳回都进入总监复核。总监是内部最终决定人，可以确认初审结论，也可以在填写原因后推翻初审结论。这样既保留两级审核，也避免单个审核员直接触发二次上门。

### 5.4 结果汇合矩阵

| 客户分支 | 内部终审 | 系统结果 |
| --- | --- | --- |
| `MANUAL_ACCEPTED` / `AUTO_ACCEPTED` | `APPROVED` | 正常结束，工单 `FINISHED` |
| `REJECTED` | 任意状态 | 立即进入二次上门，内部审核转为非阻塞质量追踪 |
| `MANUAL_ACCEPTED` / `AUTO_ACCEPTED` | `REJECTED` | 进入 `WAITING_CUSTOMER_SERVICE_CONFIRMATION`，由客服确认是否二次上门 |
| `PENDING` | `APPROVED` | 继续等待客户验收；完工满 7 天后系统自动验收 |
| `PENDING` | `REJECTED` | 仍按满 7 天自动验收，之后进入客服确认 |

任何分支都不能覆盖另一分支的原始结论。例如客户通过后，内部审核仍可以记录不合格；客户选择不再上门也不会把内部不合格改成合格。

### 5.5 客户拒绝：立即二次上门

客户拒绝验收的优先级最高，不等待审核员或总监：

```text
客户 REJECT
→ 原子创建 AFTER_SALES_CASE(type=ACCEPTANCE_REJECT)
→ 当前提交标记 CUSTOMER_REJECTED
→ 当前工单进入 SECOND_VISIT_PENDING
→ 调度确认原师傅返修或改派
→ SECOND_VISIT_IN_SERVICE
→ 新完工提交批次
→ 再次并行执行客户验收和内部两级审核
```

客户拒绝时原因必填、至少选择一个问题类型，并支持上传图片/视频。当前未结束的内部审核任务不阻塞二次上门，但允许继续以 `POST_REJECT_AUDIT` 方式完成，用于判责、师傅质量评分和防止恶意投诉。

### 5.6 客户通过但内部终审不通过

系统不能直接推翻客户验收，也不能无视平台发现的质量问题。工单进入 `WAITING_CUSTOMER_SERVICE_CONFIRMATION`，由客服主动联系客户，不由客户小程序直接决定：

1. 客服页面展示客户验收来源、评价、内部驳回原因和证据；
2. 客服联系客户，说明平台质检发现的问题；
3. 客户要求二次上门：客服选择 `SECOND_VISIT_REQUIRED`，后续完全复用“客户验收不通过”链路，创建售后案件和新的上门轮次；
4. 客户明确不需要二次上门：客服选择 `NO_SECOND_VISIT`，填写沟通记录并二次确认，工单进入 `FINISHED_WITH_REVIEW_EXCEPTION`；
5. 该场景必须同时写入结案结果 `completion_outcome=CUSTOMER_CONFIRMED_NO_SECOND_VISIT`，后台显示“客户确认无需二次上门”独立标签；
6. 客户暂时无法联系：保持客服确认状态并按 SLA 重试，不得擅自替客户选择；
7. 内部不合格结论继续影响师傅质量分、审核记录和结算，不能因为客户不要求二次上门而改为合格；
8. 客户权益和已起算质保不因内部不合格或不需要二次上门而失效。

### 5.7 二次上门模型

验收前发生的返修属于同一工单的下一个 `service_visit_cycle`，不是创建新的普通订单：

- 首次上门为 `cycle_no=1`，二次上门为 `cycle_no=2`；
- 每个轮次独立记录师傅、预约、到达、开始、证据、完工提交和审核结果；
- 可以保留原师傅，也可以由调度改派；
- 新轮次不能覆盖上一轮次的照片、评价或审核记录；
- 达到组织配置的最大免费上门次数后必须升级售后主管处理。

已正式结束后发生的质保投诉仍按第 6.5 节创建独立返修工单，不能重新打开原工单。

### 5.8 客户不操作与七天自动验收

- 自动验收基准时间为师傅本轮次提交完工的 `submitted_at`，不是内部审核完成时间；
- 提交成功时立即告知客户：“请在 7 天内验收，逾期系统将自动验收通过”；
- 第 3 天和第 6 天再次提醒客户；
- 满 7×24 小时仍未人工验收且客户没有主动驳回时，系统写入 `AUTO_ACCEPTED`；
- 人工点击通过写入 `MANUAL_ACCEPTED`，两者必须在数据库、接口、后台和小程序中明确区分；
- 自动验收后如果内部终审不通过，和人工验收通过一样进入 `WAITING_CUSTOMER_SERVICE_CONFIRMATION`；
- 所有客户统一使用 7 天自动验收规则；
- 自动验收任务必须幂等，同一提交批次只生成一条验收记录和状态历史。

## 6. 投诉与质保售后

### 6.1 投诉入口

客户在以下阶段可以投诉：

- 履约中：迟到、态度、价格、现场服务问题；
- 待验收：质量不符、资料不符、现场未完成；
- 已完成且质保有效：质量复发、配件或施工问题；
- 超出质保：仍可提交，客服判定为付费维修或驳回质保诉求。

### 6.2 投诉类型

| 类型 | 代码 | 示例 |
| --- | --- | --- |
| 施工质量 | `QUALITY` | 漏水未解决、设备仍故障 |
| 服务规范 | `SERVICE` | 迟到、态度、卫生未清理 |
| 价格争议 | `PRICING` | 未经确认加价、收费不一致 |
| 材料争议 | `MATERIAL` | 品牌/型号不符、疑似非正品 |
| 履约资料 | `EVIDENCE` | 照片不真实、质保信息缺失 |
| 其他 | `OTHER` | 需人工分类 |

### 6.3 售后案件状态

```text
NEW
→ TRIAGING
→ WAITING_CUSTOMER_INFO（可选）
→ PENDING_DECISION
→ REWORK_PENDING / COMPENSATION_PENDING / REFUND_PENDING / REJECTED
→ PROCESSING
→ WAITING_CUSTOMER_CONFIRMATION
→ RESOLVED
→ CLOSED
```

严重投诉可以进入 `ESCALATED`，并冻结关联师傅的新派单资格，但不自动停用师傅账号。

### 6.4 处理结果

- `ORIGINAL_WORKER_REWORK`：原师傅返修；
- `REASSIGN_REWORK`：改派其他师傅返修；
- `COMPENSATION`：补偿；
- `PARTIAL_REFUND`：部分退款；
- `FULL_REFUND`：全额退款；
- `PAID_REPAIR`：超保转付费维修；
- `REJECT_CLAIM`：证据不足或不在责任范围；
- `OTHER`：主管备注说明。

退款和补偿只在本 Spec 记录审批结果和金额，实际支付渠道另行实现。

### 6.5 已完成工单投诉

已 `FINISHED` 的原工单不得重新改成 `IN_SERVICE` 或 `REWORK_REQUIRED`。处理方式：

1. 创建售后案件并关联原工单、原完工提交和质保凭证；
2. 决定返修时创建新的返修工单 `REWORK_WORK_ORDER`；
3. 返修工单记录 `parent_work_order_id` 和 `after_sales_case_id`；
4. 返修工单独立派单、履约、两级审核和客户确认；
5. 原工单保持完成，售后案件汇总返修结果。

这样可以保留原履约事实，同时支持一次原工单对应多次售后服务。

## 7. 工单和提交状态机

### 7.1 为什么不能继续使用单一状态

客户可能已经验收，而审核员仍未操作；也可能内部终审不通过，但客户已经满意。若继续把这些情况都写入一个 `work_order.status`，将产生大量组合状态并互相覆盖。因此使用四个正交状态维度：

| 维度 | 字段 | 枚举 |
| --- | --- | --- |
| 作业轮次 | `visit_status` | `IN_SERVICE`、`COMPLETION_SUBMITTED`、`SECOND_VISIT_PENDING`、`SECOND_VISIT_IN_SERVICE` |
| 客户验收 | `customer_acceptance_status` | `PENDING`、`MANUAL_ACCEPTED`、`AUTO_ACCEPTED`、`REJECTED` |
| 内部审核 | `internal_review_status` | `PENDING_QA`、`PENDING_DIRECTOR`、`APPROVED`、`REJECTED`、`POST_REJECT_AUDIT` |
| 汇合/关单 | `closure_status` | `OPEN`、`WAITING_CUSTOMER_SERVICE_CONFIRMATION`、`SECOND_VISIT_PENDING`、`FINISHED`、`FINISHED_WITH_REVIEW_EXCEPTION`、`CANCELLED` |
| 结案结果 | `completion_outcome` | `NORMAL`、`SECOND_VISIT_COMPLETED`、`CUSTOMER_CONFIRMED_NO_SECOND_VISIT`、`CANCELLED` |

`completion_outcome` 在关单前为空，只能在最终关单事务中写入，写入后不可被普通业务操作修改。

对外展示的“工单状态”由上述维度按优先级派生，不作为业务真实来源。

### 7.2 完工提交批次

每次上门轮次提交完工创建不可修改的 `completion_submission`，同时初始化：

```text
customer_acceptance_status = PENDING
internal_review_status = PENDING_QA
closure_status = OPEN
```

客户验收记录、评价记录、初审记录和总监记录都关联同一个提交批次。二次上门完成后创建 `attempt_no + 1` 的新批次，旧批次只读。

### 7.3 关键转换矩阵

| 事件 | 客户状态 | 内审状态 | 汇合结果 |
| --- | --- | --- | --- |
| 师傅提交完工 | `PENDING` | `PENDING_QA` | `OPEN` |
| 客户人工通过，内审未结束 | `MANUAL_ACCEPTED` | 保持当前 | `OPEN`，展示人工验收/质检中 |
| 满 7 天自动通过，内审未结束 | `AUTO_ACCEPTED` | 保持当前 | `OPEN`，展示系统自动验收/质检中 |
| 客户驳回 | `REJECTED` | 当前任务转非阻塞 | `SECOND_VISIT_PENDING` |
| 初审完成 | 保持当前 | `PENDING_DIRECTOR` | 保持当前 |
| 总监通过，客户已通过 | `MANUAL_ACCEPTED` 或 `AUTO_ACCEPTED` | `APPROVED` | `FINISHED` |
| 总监通过，客户待处理 | `PENDING` | `APPROVED` | `OPEN`，等待客户 |
| 总监驳回，客户已通过 | `MANUAL_ACCEPTED` 或 `AUTO_ACCEPTED` | `REJECTED` | `WAITING_CUSTOMER_SERVICE_CONFIRMATION` |
| 总监驳回，客户待处理 | `PENDING` | `REJECTED` | 等待验收；满 7 天自动通过后进入客服确认 |
| 客服确认需要二次上门 | 保留历史决定 | 保留内部结论 | `SECOND_VISIT_PENDING`，复用验收驳回链路 |
| 客服确认不需要二次上门 | 保留人工/自动验收来源 | `REJECTED` | `FINISHED_WITH_REVIEW_EXCEPTION` + `CUSTOMER_CONFIRMED_NO_SECOND_VISIT` |

## 8. 数据模型

### 8.1 `completion_submission`

| 字段 | 说明 |
| --- | --- |
| `id` | 提交批次 ID |
| `org_id` | 组织 |
| `work_order_id` | 工单 |
| `attempt_no` | 第几次提交，组织+工单内递增 |
| `worker_id` | 提交师傅 |
| `summary` | 完工说明 |
| `evidence_snapshot` | 本批次证据 ID 快照或关联表 |
| `visit_cycle_id` | 所属上门轮次 |
| `customer_acceptance_status` | 客户验收分支状态 |
| `internal_review_status` | 内部审核分支状态 |
| `closure_status` | 汇合/关单状态 |
| `version` | 乐观锁 |
| `submitted_at` | 提交时间 |

约束：`UNIQUE(org_id, work_order_id, attempt_no)`。

### 8.2 `completion_review`

| 字段 | 说明 |
| --- | --- |
| `submission_id` | 完工提交批次 |
| `review_level` | `QA` / `DIRECTOR` |
| `decision` | `APPROVE` / `REJECT` |
| `reviewer_id` | 审核人 |
| `checklist_result` | 检查项结果 JSON |
| `note` | 审核意见 |
| `created_at` | 审核时间 |

约束：每个提交批次每个级别只能存在一条有效决定；纠正通过追加版本或纠正记录实现。

### 8.3 `customer_acceptance`

记录提交批次、验收决定、拒绝原因、验收来源 `MANUAL/SYSTEM_AUTO`、客户 ID、IP/设备摘要和时间。系统自动验收时操作人为空、系统任务 ID 必填。每个提交批次只允许一个最终客户决定。

### 8.4 `customer_rating`

记录提交批次、1—5 星、评价标签、评价文本、是否匿名和提交时间。验收与评价分表保存，评价缺失不能阻止验收生效。评价提交后不可更新或删除，数据库使用唯一约束保证每个提交批次最多一条评价。

### 8.5 `service_visit_cycle`

记录同一工单的每次上门：`cycle_no`、师傅、预约、到达、开始、完工时间、原因、来源提交批次和状态。客户拒绝或客服确认需要二次上门时创建下一轮次，不覆盖上一轮次。

### 8.6 `after_sales_case`

关键字段：

- `case_no`、`org_id`、`customer_id`；
- `order_id`、`work_order_id`、`submission_id`；
- `source`：客户拒绝/主动投诉/客服录入；
- `case_type`、`severity`、`status`；
- `description`、`requested_resolution`；
- `responsibility`、`resolution_type`、`resolution_note`；
- `assigned_agent_id`、`manager_id`；
- `version`、各阶段时间和关闭时间。

### 8.7 其他表

- `after_sales_evidence`：投诉图片/视频；
- `after_sales_event`：案件只追加时间线；
- `customer_service_confirmation`：客户已验收但内审不合格时，记录客服、联系时间、沟通方式、客户是否需要二次上门、备注和二次确认信息；
- `rework_work_order_relation`：原工单、返修工单、售后案件关系；
- `warranty_certificate`：验收完成后生效的质保凭证；
- `employee_role_assignment`：一个员工可有多个后台角色，但审核时执行职责隔离校验。

### 8.8 工单结案标记与查询字段

`work_order` 增加或维护以下可索引字段：

| 字段 | 说明 |
| --- | --- |
| `completion_outcome` | 结案结果；本场景固定为 `CUSTOMER_CONFIRMED_NO_SECOND_VISIT` |
| `closed_at` | 工单最终关闭时间 |
| `closed_by` | 执行关闭的客服账号 |
| `customer_service_confirmation_id` | 对应客服确认记录 |
| `has_review_exception` | 是否存在内部终审不合格 |

必须建立组织维度索引：`(org_id, completion_outcome, closed_at DESC)`，保证运营能够直接筛选，不允许通过解析备注或遍历时间线推断。

## 9. 后台管理设计

### 9.1 履约调度页

增加独立队列：

- 待审核员初审；
- 待总监复核；
- 客户已通过但内审未完成；
- 客户已驳回并待二次上门；
- 客户已通过、内审不通过且待客服确认；
- 内审已通过但客户待验收；
- 验收逾期；
- 待返修；
- 投诉处理中。

列表增加“结案结果”筛选项：

- 全部；
- 正常完结；
- 二次上门后完结；
- 客户确认无需二次上门；
- 已取消。

选择“客户确认无需二次上门”时，后端按 `completion_outcome=CUSTOMER_CONFIRMED_NO_SECOND_VISIT` 精确查询。列表行展示独立橙色标签“客户确认无需二次上门”，并同时保留“人工验收/自动验收”和“内部审核不通过”标识。

### 9.2 审核员初审弹窗

展示：

- 工单、客户、SKU、师傅和预约信息；
- 本次提交的完工说明；
- 按 `BEFORE/DURING/AFTER/TEST` 分组的图片和视频；
- SKU 对应检查清单；
- 历史提交和驳回次数。
- 客户验收状态、评分、标签和文字评价；若客户尚未操作则显示“客户待验收”。

操作只有“通过”“驳回”。驳回必须选择问题项并填写原因，提交前二次确认。

### 9.3 总监复核弹窗

除初审内容外，还必须展示：

- 初审员姓名、时间、检查项和意见；
- 客户验收决定、评分、评价内容和客户上传的问题证据；
- 工单金额、返工次数、投诉历史和风险标签；
- 图片重复/缺失等系统风险提示。

总监通过和驳回都需要二次确认；驳回原因必填。

### 9.4 售后案件页

包含案件列表、详情、证据、沟通时间线、判责、处理方案、返修工单和客户确认。高风险和超时案件需突出显示。

### 9.5 完整工单流程时间线

工单详情必须提供统一时间线，按时间和事件序号展示整个过程，不只展示当前状态。至少包含：

- 下单、确认、派单、改派和预约；
- 每次上门轮次的接单、到达、开始服务；
- 每个完工提交批次、完工说明和证据快照；
- 客户人工/系统自动验收、一次性评价；
- 审核员初审和总监终审；
- 进入客服确认、客服沟通记录和确认结果；
- 创建二次上门或客户确认无需二次上门；
- 投诉、售后处理、关单和质保生效。

每个事件展示时间、事件名称、操作角色、操作人、前后状态、公开说明和内部备注。内部备注只对有权限的后台角色可见。时间线由只追加事件表和业务记录聚合生成，不允许前端根据当前状态伪造历史步骤。

## 10. 小程序设计

### 10.1 师傅作业小程序

- 提交完工后显示“客户验收中 / 平台质检中”两个并行进度；
- 初审通过后显示“等待总监复核”；
- 任一级驳回都展示驳回级别、公开给师傅的原因和问题项；
- 返修后创建新提交批次，不能编辑旧批次；
- 客户投诉处理中显示售后状态，但不展示内部判责备注。

### 10.2 客户小程序

- 师傅提交完工后立即展示验收和评价入口，不等待内部审核；
- 验收页面展示当前完工提交及客户可见证据；
- “拒绝验收”必须选择问题类型、填写原因，可上传证据；
- 客户已经通过而内部终审不通过时，展示“平台质检发现问题，客服将与您确认”的提示；
- 客户端只展示“客服确认中”，最终是否二次上门由客服与客户沟通后在后台记录；
- 人工验收显示“客户已确认”，自动验收显示“系统7天自动验收”，不能统一显示为“已验收”；
- 订单详情提供“投诉/申请售后”入口；
- 售后页展示受理、处理方案、返修进度和结果确认；
- 客户不能查看审核员/总监的内部意见和责任判定草稿。

## 11. API 设计

### 11.1 师傅端

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `POST` | `/api/v1/worker/work-orders/{id}/submit-completion` | 创建新的完工提交批次 |
| `GET` | `/api/v1/worker/work-orders/{id}/completion-submissions` | 查看自己的提交和公开驳回意见 |
| `POST` | `/api/v1/worker/work-orders/{id}/start-rework` | 开始返修 |

### 11.2 审核员与总监

| 方法 | 路径 | 权限 | 说明 |
| --- | --- | --- | --- |
| `GET` | `/api/v1/admin/qa-reviews` | `QA_REVIEWER` | 初审队列 |
| `POST` | `/api/v1/admin/completion-submissions/{id}/qa-review` | `QA_REVIEWER` | 初审通过/驳回 |
| `GET` | `/api/v1/admin/director-reviews` | `QA_DIRECTOR` | 总监复核队列 |
| `POST` | `/api/v1/admin/completion-submissions/{id}/director-review` | `QA_DIRECTOR` | 总监通过/驳回 |
| `GET` | `/api/v1/admin/completion-submissions/{id}` | 相应审核角色 | 提交、证据和审核详情 |

所有审核命令必须携带 `Idempotency-Key` 和 `version`。

审核详情响应必须包含只读的 `customerAcceptance` 和 `customerRating`。客户在审核页面打开后又提交验收或评价时，审核提交接口必须基于最新版本校验并提示刷新，避免审核人员基于过期客户反馈作出决定。

### 11.3 客户验收与投诉

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `POST` | `/api/v1/mini/work-orders/{id}/acceptance` | 客户通过或拒绝验收 |
| `POST` | `/api/v1/mini/work-orders/{id}/rating` | 一次性提交评价，提交后不可修改 |
| `POST` | `/api/v1/mini/after-sales-cases` | 主动发起投诉/售后 |
| `GET` | `/api/v1/mini/after-sales-cases` | 客户案件列表 |
| `GET` | `/api/v1/mini/after-sales-cases/{id}` | 客户案件详情 |
| `POST` | `/api/v1/mini/after-sales-cases/{id}/evidence` | 补充证据 |
| `POST` | `/api/v1/mini/after-sales-cases/{id}/confirmation` | 确认或拒绝售后结果 |

### 11.4 售后后台

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/api/v1/admin/after-sales-cases` | 案件队列 |
| `GET` | `/api/v1/admin/after-sales-cases/{id}` | 案件详情和时间线 |
| `POST` | `/api/v1/admin/after-sales-cases/{id}/assign` | 指派客服 |
| `POST` | `/api/v1/admin/after-sales-cases/{id}/triage` | 分类和严重度 |
| `POST` | `/api/v1/admin/after-sales-cases/{id}/resolution` | 提交处理方案 |
| `POST` | `/api/v1/admin/after-sales-cases/{id}/approve-resolution` | 售后主管审批方案 |
| `POST` | `/api/v1/admin/after-sales-cases/{id}/create-rework` | 创建返修工单 |
| `POST` | `/api/v1/admin/after-sales-cases/{id}/close` | 关闭案件 |
| `POST` | `/api/v1/admin/work-orders/{id}/customer-service-confirmation` | 客服确认是否二次上门；需要时复用验收驳回链路 |

### 11.5 工单筛选与完整时间线

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/api/v1/admin/work-orders?completionOutcome=CUSTOMER_CONFIRMED_NO_SECOND_VISIT` | 筛选客户确认无需二次上门的工单 |
| `GET` | `/api/v1/admin/work-orders/{id}/timeline` | 获取完整工单流程时间线 |

列表响应必须返回 `completionOutcome`、`closureStatus`、`customerAcceptanceStatus`、`internalReviewStatus` 和 `hasReviewException`，避免前端根据文案猜测标签。

## 12. 通知与时效

| 事件 | 通知对象 | 时效建议 |
| --- | --- | --- |
| 师傅提交完工 | 客户、初审队列 | 同时即时通知 |
| 初审超时 | 初审员/主管 | 2 小时提醒，4 小时升级 |
| 初审通过 | 总监队列 | 即时 |
| 总监复核超时 | 总监/运营负责人 | 4 小时提醒，8 小时升级 |
| 内部终审驳回且客户已验收 | 客服队列 | 即时，客服确认是否二次上门 |
| 完工待验收 | 客户 | 提交时告知7天规则，第3天、第6天提醒 |
| 七天自动验收 | 客户、审核后台 | 即时，明确标记系统自动验收 |
| ToC 待验收 | 客户 | 24、48 小时提醒 |
| 客户拒绝/投诉 | 客服和售后主管 | 即时；严重投诉升级 |
| 售后案件超时 | 客服、主管、运营负责人 | 按严重度 SLA |

首期通知可以先使用站内待办，微信订阅消息后续接入。

## 13. 订单汇总与质保

- 客户已验收但内部审核未结束时，订单显示“客户已验收/平台质检中”，不回退客户结果。
- 任一工单等待客户决定、二次上门或存在阻断型售后案件时，订单不能最终关闭。
- 订单全部工单为 `FINISHED` 或 `FINISHED_WITH_REVIEW_EXCEPTION` 后转为 `COMPLETED`；后者保留内部质量异常标识。
- 已完成订单发生新投诉时，订单历史状态不回退；订单详情增加“售后处理中”派生标识。
- 质保从客户验收或系统自动验收时间开始计算。
- 返修不默认重置整单质保；是否延长对应维修部位质保由 SKU/合同规则决定并记录。

## 14. 并发、幂等与审计

- 提交完工、两级审核、客户验收、创建投诉、判责、创建返修均要求 `Idempotency-Key`。
- 所有写操作携带 `version`，数据库事务内锁定工单/提交/案件当前行。
- 相同幂等键和相同请求返回首次结果；相同键不同请求返回 `IDEMPOTENCY_KEY_CONFLICT`。
- 状态更新和事件历史必须同事务提交。
- 审核、客户验收、售后判责和退款方案均只追加历史，不允许物理删除。
- 媒体访问每次校验组织、业务归属和角色；客户只能读取 `customer_visible=true` 的证据。
- 管理员导出审计记录需要单独权限并记录导出日志。

## 15. 业务错误码

| 错误码 | HTTP | 场景 |
| --- | --- | --- |
| `QA_REVIEW_REQUIRED` | 409 | 尚未完成审核员初审 |
| `DIRECTOR_REVIEW_REQUIRED` | 409 | 尚未完成总监复核 |
| `REVIEW_ROLE_FORBIDDEN` | 403 | 当前账号没有对应审核角色 |
| `REVIEWER_CONFLICT` | 409 | 初审员与总监相同或审核人为执行师傅 |
| `SUBMISSION_ALREADY_REVIEWED` | 409 | 当前提交批次已完成该级审核 |
| `SUBMISSION_NOT_CURRENT` | 409 | 操作的不是工单当前提交批次 |
| `REVIEW_CHECKLIST_INCOMPLETE` | 400 | 必填检查项未完成 |
| `REVIEW_REASON_REQUIRED` | 400 | 驳回原因缺失 |
| `ACCEPTANCE_REASON_REQUIRED` | 400 | 客户拒绝原因缺失 |
| `CUSTOMER_SERVICE_CONFIRMATION_REQUIRED` | 409 | 客户已验收但内审不合格，等待客服确认 |
| `CUSTOMER_SERVICE_NOTE_REQUIRED` | 400 | 客服确认缺少沟通记录 |
| `RATING_ALREADY_SUBMITTED` | 409 | 客户评价已提交，不允许修改或重复提交 |
| `AFTER_SALES_CASE_EXISTS` | 409 | 同一问题已有进行中案件 |
| `WARRANTY_EXPIRED` | 409 | 质保已过期，可转付费处理 |
| `RESOLUTION_APPROVAL_REQUIRED` | 409 | 处理方案尚未由主管批准 |
| `RESOURCE_VERSION_CONFLICT` | 409 | 数据已被其他操作修改 |
| `IDEMPOTENCY_KEY_CONFLICT` | 409 | 幂等键被用于不同请求 |

## 16. 数据迁移与兼容

上线时新增向前 migration，不修改已执行 migration：

- 增加作业轮次、客户验收、内部审核和汇合关单四组状态字段；
- 新增完工提交、两级审核、客户验收、客户评价、上门轮次、售后案件和返修关系表；
- 为现有员工增加多角色关联；
- 将 `WAITING_COMPLETION_REVIEW` 迁移为 `customer_acceptance_status=PENDING`、`internal_review_status=PENDING_QA`；迁移完成后客户立即可验收；
- 已处于 `WAITING_ACCEPTANCE` 的存量工单生成 `LEGACY_MIGRATION` 内审通过记录，并设置客户状态为 `PENDING`；
- 历史 `REWORK_REQUIRED` 工单保留，并在下一次提交时创建首个正式提交批次；
- 不修改已完成工单的原状态和完成时间。

## 17. 验收场景

### AC-007-01｜并行启动

Given 师傅提交了完整完工资料  
When 提交事务完成  
Then 客户立即看到验收和评价入口，同时审核员看到初审任务，两者互不等待。

### AC-007-02｜审核感知客户反馈

Given 客户已验收并评价  
When 审核员和总监打开当前提交  
Then 页面展示客户决定、评分、标签和评价内容，并且审核记录保存所依据的客户反馈版本。

### AC-007-03｜客户通过且内审通过

Given 客户已经通过验收  
When 审核员完成初审且总监终审通过  
Then 工单进入 `FINISHED`，客户验收时间作为质保起算时间。

### AC-007-04｜职责隔离

Given 审核员已经初审通过  
When 同一账号尝试执行总监复核，或执行师傅尝试审核  
Then 返回 `REVIEWER_CONFLICT`，不产生状态历史。

### AC-007-05｜客户拒绝立即二次上门

Given 师傅已提交完工且内部审核处于任意阶段  
When 客户填写问题、上传证据并拒绝验收  
Then 系统立即创建售后案件和二次上门轮次，不等待内部审核完成。

### AC-007-06｜已完成后投诉

Given 工单已经完成且仍在质保期  
When 客户发起质量投诉并判定返修  
Then 原工单保持 `FINISHED`，系统创建关联返修工单并独立履约。

### AC-007-07｜客户通过但内审不通过

Given 客户已通过而总监终审不通过  
When 客服联系客户并提交确认结果  
Then 客户需要二次上门时复用验收驳回链路；客户不需要时工单以 `FINISHED_WITH_REVIEW_EXCEPTION` 结束。

### AC-007-08｜七天自动验收来源可区分

Given 师傅提交完工后客户一直未操作  
When 满 7×24 小时  
Then 系统只生成一次 `AUTO_ACCEPTED` 记录；人工通过则为 `MANUAL_ACCEPTED`，前后台均能明确区分。

### AC-007-09｜评价不可修改

Given 客户已提交评价  
When 再次提交或尝试修改评价  
Then 返回 `RATING_ALREADY_SUBMITTED`，原评价保持不变。

### AC-007-10｜并发和幂等

Given 两个审核请求同时提交  
When 请求版本相同或复用幂等键  
Then 只产生一个有效决定和一条对应状态历史。

### AC-007-11｜媒体越权

Given 客户、师傅和审核人员属于不同工单或组织  
When 请求不属于自己的证据  
Then 返回 403/404，不泄露媒体内容和元数据。

### AC-007-12｜端到端

在真实 PostgreSQL 环境完成：

```text
后台派单
→ 师傅履约并提交
→ 客户验收/评价与审核员初审并行
→ 客户先通过
→ 总监终审驳回
→ 进入客服确认
→ 客服确认客户需要二次上门
→ 新轮次履约并提交
→ 客户通过且内部终审通过
→ 工单完成和质保生效
```

数据库、Go API、React 后台、客户小程序和师傅小程序均需保留可重复验证证据。

### AC-007-13｜无需二次上门标记与追溯

Given 客户已验收、内部终审不通过且客服确认客户不需要二次上门  
When 客服二次确认并关闭工单  
Then 工单写入 `completion_outcome=CUSTOMER_CONFIRMED_NO_SECOND_VISIT`，履约调度可按该条件筛选，并能在详情时间线看到从派单、履约、验收、评价、两级审核到客服确认和关单的完整记录。

## 18. 实施优先级

### P0｜必须先完成

- 完工提交批次；
- 客户验收/评价与内部审核并行状态模型；
- 审核员/总监角色和职责隔离；
- 两级审核状态机；
- 四种结果汇合和客服确认；
- 客户确认无需二次上门的独立标记、索引和筛选；
- 完整工单流程时间线；
- 人工验收/七天自动验收来源区分；
- 一次性不可修改评价；
- 二次上门轮次；
- 客户拒绝验收自动建立售后案件；
- 后台两级审核队列和小程序状态展示；
- PostgreSQL 并发、幂等和越权测试。

### P1｜紧接完成

- 售后案件后台；
- 原师傅返修/改派返修；
- 已完成工单投诉创建返修工单；
- 自动验收提醒任务；
- 质保凭证生效规则。

### P2｜后续增强

- 图片重复和异常风险识别；
- 高风险工单自动升级指定总监；
- 微信订阅消息；
- SLA 看板、投诉率、驳回率和审核员质量指标；
- 退款支付和财务结算联动。

## 19. 待业务确认项

以下参数不阻塞状态机开发，但上线前需要运营确认：

- 初审和总监复核 SLA；
- 投诉严重度分级及升级联系人；
- 退款/补偿金额的分级审批阈值；
- 各 SKU 必传证据模板和质保期限；
- 返修后质保延长规则。
