import { listWorkOrders, type WorkOrder } from '../../services/work-orders'
import { workOrderStatusLabel } from '../../services/status'

Page({
  data: { loading: false, error: '', items: [] as WorkOrder[] },
  onShow() { this.load() },
  open(e: WechatMiniprogram.TouchEvent) {
    const id = (e.currentTarget as any).dataset.id as string
    wx.navigateTo({ url: `/pages/work-order-detail/index?id=${id}` })
  },
  async load() {
    this.setData({ loading: true, error: '' })
    try {
      const result = await listWorkOrders()
      this.setData({ items: (result.items ?? []).map(item => ({ ...item, statusText: workOrderStatusLabel(item.status) })) })
    } catch (error) {
      this.setData({ error: error instanceof Error ? error.message : '工单加载失败' })
    } finally {
      this.setData({ loading: false })
    }
  },
})
