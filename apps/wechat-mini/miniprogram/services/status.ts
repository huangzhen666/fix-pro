const labels: Record<string, string> = {
  PENDING_PAYMENT: '待支付', PENDING_CONFIRMATION: '待确认', FULFILLING: '履约中', WAITING_ACCEPTANCE: '待客户验收', COMPLETED: '已完成', CANCELLED: '已取消',
  PENDING_DISPATCH: '待派单', PENDING_ACCEPT: '待师傅接单', PENDING_ARRIVAL: '待上门', ARRIVED: '已到达', IN_SERVICE: '服务中',
  WAITING_COMPLETION_REVIEW: '待审核', WAITING_QA_AUDIT: '待质检初审', WAITING_DIRECTOR_AUDIT: '待总监审核', WAITING_CUSTOMER_SERVICE_CONFIRMATION: '待客服确认',
  SECOND_VISIT_PENDING: '待二次上门', REWORK_REQUIRED: '待返工', FINISHED: '已完成', FINISHED_WITH_REVIEW_EXCEPTION: '审核异常已完结',
  BEFORE: '施工前', DURING: '施工中', AFTER: '施工后', MANUAL: '人工验收', AUTO: '系统自动验收', MANUAL_ACCEPTED: '人工验收通过', AUTO_ACCEPTED: '系统自动验收通过',
}
export function statusLabel(value?: string | null): string { return value ? (labels[value] ?? value) : '-' }
