import { getCart } from '../../services/cart'
import { listAddresses, type CustomerAddress } from '../../services/addresses'
import { listCustomerOrders } from '../../services/orders'

type OrderCounts = { pendingPayment: number; pendingService: number; inService: number; pendingRating: number; returned: number }

const emptyOrderCounts: OrderCounts = { pendingPayment: 0, pendingService: 0, inService: 0, pendingRating: 0, returned: 0 }

function countOrders(items: Array<{ status: string; cancelReason?: string }>): OrderCounts {
  return {
    pendingPayment: items.filter(item => item.status === 'PENDING_PAYMENT').length,
    pendingService: items.filter(item => item.status === 'PENDING_CONFIRMATION').length,
    inService: items.filter(item => item.status === 'FULFILLING').length,
    pendingRating: items.filter(item => item.status === 'COMPLETED').length,
    returned: items.filter(item => item.status === 'CANCELLED' && !!item.cancelReason).length,
  }
}

Page({
  data: { cartCount: 0, defaultAddress: null as CustomerAddress | null, orderCounts: emptyOrderCounts },
  onShow() {
    Promise.all([getCart(), listAddresses()]).then(([cart, addresses]) => this.setData({ cartCount: cart.itemCount, defaultAddress: addresses.find(item => item.isDefault) || addresses[0] || null })).catch(() => {})
    listCustomerOrders().then(result => this.setData({ orderCounts: countOrders(result.items) })).catch(() => {})
  },
  openCart() { wx.navigateTo({ url: '/pages/cart/index' }) },
  openOrders(e?: any) { const status = e?.currentTarget?.dataset?.status; wx.navigateTo({ url: status ? `/pages/orders/index?status=${status}` : '/pages/orders/index' }) },
  openAddresses() { wx.navigateTo({ url: '/pages/addresses/index' }) },
  placeholder(e: any) { wx.showToast({ title: `${e.currentTarget.dataset.name}将在后续版本开放`, icon: 'none' }) },
  contact() { wx.showModal({ title: '联系客服', content: '本地演示环境暂未配置企业微信或客服电话。', showCancel: false }) },
})
