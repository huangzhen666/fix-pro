const labels: Record<string, string> = {
  PENDING_ACCEPT: '待接单', PENDING_ARRIVAL: '待上门', ARRIVED: '已到达', IN_SERVICE: '服务中',
  WAITING_COMPLETION_REVIEW: '待审核', WAITING_QA_AUDIT: '待质检初审', WAITING_DIRECTOR_AUDIT: '待总监审核', WAITING_CUSTOMER_SERVICE_CONFIRMATION: '待客服确认', WAITING_ACCEPTANCE: '待客户验收', REWORK_REQUIRED: '待返工', SECOND_VISIT_PENDING: '待二次上门', FINISHED_WITH_REVIEW_EXCEPTION: '审核异常已完结', FINISHED: '已完成',
  CANCELLED: '已取消', PENDING_DISPATCH: '待派单',
}

export function workOrderStatusLabel(status: string): string { return labels[status] ?? status }
