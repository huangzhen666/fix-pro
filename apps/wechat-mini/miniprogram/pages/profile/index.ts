import { getCart } from '../../services/cart'
import { listAddresses, type CustomerAddress } from '../../services/addresses'
import { listCustomerOrders, type CustomerOrderSummary } from '../../services/orders'

type OrderCounts = { pendingPayment: number; pendingService: number; inService: number; pendingRating: number; returned: number }
type TrackedOrderStatus = 'PENDING_PAYMENT' | 'PENDING_CONFIRMATION' | 'FULFILLING' | 'COMPLETED' | 'CANCELLED'
type ReadOrderState = { keys: string[]; viewedAt: Partial<Record<TrackedOrderStatus, number>> }

const emptyOrderCounts: OrderCounts = { pendingPayment: 0, pendingService: 0, inService: 0, pendingRating: 0, returned: 0 }
const readOrderStateKey = 'profile.read-order-shortcuts'
const countKeyByStatus: Record<TrackedOrderStatus, keyof OrderCounts> = {
  PENDING_PAYMENT: 'pendingPayment',
  PENDING_CONFIRMATION: 'pendingService',
  FULFILLING: 'inService',
  COMPLETED: 'pendingRating',
  CANCELLED: 'returned',
}
let currentOrders: CustomerOrderSummary[] = []

function getReadOrderState(): ReadOrderState {
  const stored = wx.getStorageSync(readOrderStateKey)
  if (!stored || typeof stored !== 'object') return { keys: [], viewedAt: {} }
  const value = stored as { keys?: unknown; viewedAt?: unknown }
  return {
    keys: Array.isArray(value.keys) ? value.keys.filter((key): key is string => typeof key === 'string') : [],
    viewedAt: value.viewedAt && typeof value.viewedAt === 'object' ? value.viewedAt as ReadOrderState['viewedAt'] : {},
  }
}

function isRead(item: CustomerOrderSummary, status: TrackedOrderStatus, state: ReadOrderState): boolean {
  if (state.keys.includes(`${status}:${item.id}`)) return true
  const viewedAt = state.viewedAt[status]
  const statusUpdatedAt = Date.parse(item.statusUpdatedAt || item.createdAt)
  return typeof viewedAt === 'number' && Number.isFinite(statusUpdatedAt) && statusUpdatedAt <= viewedAt
}

function countOrders(items: CustomerOrderSummary[]): OrderCounts {
  const readState = getReadOrderState()
  const countUnread = (status: TrackedOrderStatus, predicate: (item: CustomerOrderSummary) => boolean) => items.filter(item => item.status === status && predicate(item) && !isRead(item, status, readState)).length
  return {
    pendingPayment: countUnread('PENDING_PAYMENT', () => true),
    pendingService: countUnread('PENDING_CONFIRMATION', () => true),
    inService: countUnread('FULFILLING', () => true),
    pendingRating: countUnread('COMPLETED', () => true),
    returned: countUnread('CANCELLED', item => !!item.cancelReason),
  }
}

function markOrderStatusAsRead(status: TrackedOrderStatus) {
  const state = getReadOrderState()
  const keys = new Set(state.keys)
  currentOrders.filter(item => item.status === status).forEach(item => keys.add(`${status}:${item.id}`))
  state.keys = Array.from(keys).slice(-1000)
  state.viewedAt[status] = Date.now()
  wx.setStorageSync(readOrderStateKey, state)
}

Page({
  data: { cartCount: 0, defaultAddress: null as CustomerAddress | null, orderCounts: emptyOrderCounts },
  onShow() {
    Promise.all([getCart(), listAddresses()]).then(([cart, addresses]) => this.setData({ cartCount: cart.itemCount, defaultAddress: addresses.find(item => item.isDefault) || addresses[0] || null })).catch(() => {})
    listCustomerOrders().then(result => {
      currentOrders = result.items
      this.setData({ orderCounts: countOrders(result.items) })
    }).catch(() => {})
  },
  openCart() { wx.navigateTo({ url: '/pages/cart/index' }) },
  openOrders(e?: any) {
    const status = e?.currentTarget?.dataset?.status as string | undefined
    if (status && status in countKeyByStatus) {
      const trackedStatus = status as TrackedOrderStatus
      markOrderStatusAsRead(trackedStatus)
      this.setData({ orderCounts: { ...this.data.orderCounts, [countKeyByStatus[trackedStatus]]: 0 } })
    }
    wx.navigateTo({ url: status ? `/pages/orders/index?status=${status}` : '/pages/orders/index' })
  },
  openAddresses() { wx.navigateTo({ url: '/pages/addresses/index' }) },
  placeholder(e: any) { wx.showToast({ title: `${e.currentTarget.dataset.name}将在后续版本开放`, icon: 'none' }) },
  contact() { wx.showModal({ title: '联系客服', content: '本地演示环境暂未配置企业微信或客服电话。', showCancel: false }) },
})
