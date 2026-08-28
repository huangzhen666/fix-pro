import { listCustomerOrders, type CustomerOrderSummary } from '../../services/orders'
import { statusLabel } from '../../services/status'

let currentStatusFilter = ''

Page({
  data: { loading: true, statusFilter: '', items: [] as (CustomerOrderSummary & { amountText: string })[] },

  onLoad(options: Record<string, string>) {
    currentStatusFilter = options?.status || ''
    this.setData({ statusFilter: currentStatusFilter })
  },

  onShow() {
    this.load(currentStatusFilter)
  },

  async load(statusFilter = currentStatusFilter) {
    this.setData({ loading: true })
    try {
      const result = await listCustomerOrders(statusFilter)
      const items = result.items.map(item => ({
        ...item,
        statusText: item.statusText || statusLabel(item.status),
        amountText: (item.totalAmount / 100).toFixed(2),
      }))
      this.setData({ items })
    } catch (e) {
      wx.showToast({ title: e instanceof Error ? e.message : '订单加载失败', icon: 'none' })
    } finally {
      this.setData({ loading: false })
    }
  },

  openDetail(e: any) {
    wx.navigateTo({ url: `/pages/orders/detail?id=${e.currentTarget.dataset.id}` })
  },
})
