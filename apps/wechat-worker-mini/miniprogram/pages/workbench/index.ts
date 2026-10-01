import { listWorkOrders, type WorkOrder } from '../../services/work-orders'

const summaryDefinitions = [
  { label: '待接单', status: 'PENDING_ACCEPT' },
  { label: '待上门', status: 'PENDING_ARRIVAL' },
  { label: '服务中', status: 'IN_SERVICE' },
  { label: '待完工', status: 'WAITING_COMPLETION_REVIEW' },
]

function summarize(items: WorkOrder[]) {
  return summaryDefinitions.map(({ label, status }) => ({ label, value: items.filter(item => item.status === status).length }))
}

Page({
  data: { summary: summarize([]) },
  onShow() {
    wx.setNavigationBarTitle({ title: '师傅工作台' })
    this.load()
  },
  async load() {
    try {
      const result = await listWorkOrders()
      this.setData({ summary: summarize(result.items ?? []) })
    } catch {
      wx.showToast({ title: '待办加载失败', icon: 'none' })
    }
  },
})
