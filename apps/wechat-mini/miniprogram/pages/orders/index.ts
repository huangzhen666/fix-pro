import { listCustomerOrders, type CustomerOrderSummary } from '../../services/orders'
import { statusLabel } from '../../services/status'

Page({ data: { loading: true, items: [] as (CustomerOrderSummary & { amountText: string })[] }, onShow() { this.load() }, async load() { this.setData({ loading: true }); try { const result = await listCustomerOrders(); const items = result.items.map(item => ({ ...item, statusText: statusLabel(item.status), amountText: (item.totalAmount / 100).toFixed(2) })); this.setData({ items }) } catch (e) { wx.showToast({ title: e instanceof Error ? e.message : '订单加载失败', icon: 'none' }) } finally { this.setData({ loading: false }) } }, openDetail(e: any) { wx.navigateTo({ url: `/pages/orders/detail?id=${e.currentTarget.dataset.id}` }) } })
