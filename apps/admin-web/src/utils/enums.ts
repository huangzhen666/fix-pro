const labels: Record<string, string> = {
  ACTIVE: '启用', DISABLED: '停用', DRAFT: '草稿', PUBLISHED: '已发布', OFF_SHELF: '已下架',
  PENDING_PAYMENT: '待支付', PENDING_CONFIRMATION: '待确认', FULFILLING: '履约中', WAITING_ACCEPTANCE: '待验收', COMPLETED: '已完成', CANCELLED: '已取消',
  PENDING_DISPATCH: '待派单', PENDING_ACCEPT: '待接单', PENDING_ARRIVAL: '待上门', ARRIVED: '已到达', IN_SERVICE: '服务中', WAITING_COMPLETION_REVIEW: '待完工审核', COMPLETION_SUBMITTED: '已提交完工', COMPLETION_APPROVED: '完工审核通过', COMPLETION_REJECTED: '完工审核驳回', WAITING_QA_AUDIT: '待质检初审', WAITING_DIRECTOR_AUDIT: '待总监审核', WAITING_CUSTOMER_SERVICE_CONFIRMATION: '待客服确认', SECOND_VISIT_PENDING: '待二次上门', FINISHED_WITH_REVIEW_EXCEPTION: '审核异常已完结', FINISHED: '已完成', REWORK_REQUIRED: '待返工',
  NORMAL: '普通', URGENT: '紧急', FIXED: '固定价', UP: '正常', DOWN: '异常',
}
export const enumLabel = (value?: string | null) => value ? (labels[value] ?? value) : '-'
export const statusLabel = enumLabel
export const orderStatusLabel = enumLabel
export const workOrderStatusLabel = enumLabel
export const adminWorkOrderStatusLabel = (status?: string | null, customerAcceptanceStatus?: string | null) => customerAcceptanceStatus && ['MANUAL_ACCEPTED', 'AUTO_ACCEPTED'].includes(customerAcceptanceStatus) && (status === 'WAITING_QA_AUDIT' || status === 'WAITING_DIRECTOR_AUDIT') ? '已完成待审核' : enumLabel(status)
export const priorityLabel = enumLabel
