import { listWorkOrders, type WorkOrder } from '../../services/work-orders'
import { workOrderStatusLabel } from '../../services/status'

const weekNames = ['日', '一', '二', '三', '四', '五', '六']

function formatAppointmentDate(value?: string) {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return `${date.getFullYear()}年${String(date.getMonth() + 1).padStart(2, '0')}月${String(date.getDate()).padStart(2, '0')}日（周${weekNames[date.getDay()]}）`
}

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
      this.setData({ items: (result.items ?? []).map(item => ({ ...item, statusText: workOrderStatusLabel(item.status), appointmentDateText: formatAppointmentDate(item.appointmentAt) })) })
    } catch (error) {
      this.setData({ error: error instanceof Error ? error.message : '工单加载失败' })
    } finally {
      this.setData({ loading: false })
    }
  },
})
