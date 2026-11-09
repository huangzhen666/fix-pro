import { protectedPage } from '../../services/guard'
import { getCart, confirmCartItem, removeItem, updateQuantity, type Cart, type CartItem } from '../../services/cart'
import { mediaUrl } from '../../services/presentation'
import { onSessionReset } from '../../services/session'

const checkoutItemsStorageKey = 'fixpro.checkoutCartItemIds'
type DisplayCartItem = CartItem & { unitPriceText: string; selected: boolean }
type DisplayCart = Omit<Cart, 'items'> & { items: DisplayCartItem[] }
type Gesture = { id: string; x: number; y: number; offset: number; horizontal: boolean; vertical: boolean }
type State = { revision: number; hidden: boolean; dead: boolean; selected: Record<string, boolean>; gesture?: Gesture; suppressUntil: number; navigating: boolean }
const states = new WeakMap<object, State>()
const activePages = new Set<any>()
function state(page: object): State {
  let value = states.get(page)
  if (!value) { value = { revision: 0, hidden: false, dead: false, selected: {}, suppressUntil: 0, navigating: false }; states.set(page, value) }
  return value
}
function current(page: object, revision: number) { const s = state(page); return !s.dead && !s.hidden && s.revision === revision }
function summary(items: DisplayCartItem[]) {
  const available = items.filter(item => item.available)
  const selected = available.filter(item => item.selected)
  return { totalText: (selected.reduce((sum, item) => sum + item.subtotal, 0) / 100).toFixed(2), selectedCount: selected.length, allSelected: !!available.length && selected.length === available.length }
}
onSessionReset(() => {
  activePages.forEach(page => {
    const s = state(page); s.revision++; s.selected = {}; s.gesture = undefined
    page.setData({ cart: { items: [], itemCount: 0, totalAmount: 0 }, ...summary([]), loading: false, loadError: '', updatingId: '', checkoutLoading: false, swipeId: '', swipeOffset: 0 })
  })
})

protectedPage({
  data: {
    loading: true, loadError: '', cart: { items: [], itemCount: 0, totalAmount: 0 } as DisplayCart,
    totalText: '0.00', selectedCount: 0, allSelected: false, updatingId: '', checkoutLoading: false,
    swipeId: '', swipeOffset: 0, swipeWidth: 72, dragging: false,
  },
  onLoad() { activePages.add(this); this.setData({ swipeWidth: wx.getWindowInfo().windowWidth * 144 / 750 }) },
  onShow() { state(this).hidden = false; this.load() },
  onHide() {
    const s = state(this); s.hidden = true; s.revision++; s.gesture = undefined
    this.setData({ loading: false, updatingId: '', checkoutLoading: false, swipeId: '', swipeOffset: 0, dragging: false })
  },
  onPullDownRefresh() { this.load().finally(() => wx.stopPullDownRefresh()) },
  onUnload() { const s = state(this); s.dead = true; s.revision++; s.selected = {}; activePages.delete(this) },
  applyCart(cart: Cart) {
    const choices = state(this).selected
    const items = cart.items.map(item => ({ ...item, coverImageUrl: mediaUrl(item.coverImageUrl), unitPriceText: (item.unitPrice / 100).toFixed(2), selected: item.available && (choices[item.id] ?? true) }))
    this.setData({ cart: { ...cart, items }, ...summary(items), loadError: '', swipeId: '', swipeOffset: 0, dragging: false })
  },
  async load() {
    if (this.data.updatingId || this.data.checkoutLoading || state(this).dead || state(this).hidden) return
    activePages.add(this)
    const revision = ++state(this).revision
    this.setData({ loading: true, loadError: '' })
    try { const cart = await getCart(); if (current(this, revision)) this.applyCart(cart) }
    catch (error) { if (current(this, revision)) this.setData({ loadError: error instanceof Error ? error.message : '购物车加载失败，请重试' }) }
    finally { if (current(this, revision)) this.setData({ loading: false }) }
  },
  toggleSelected(e: any) {
    const item = this.data.cart.items.find(entry => entry.id === String(e.currentTarget.dataset.id))
    if (!item?.available || this.data.checkoutLoading) return
    state(this).selected[item.id] = !item.selected
    const items = this.data.cart.items.map(entry => ({ ...entry, selected: entry.available && (state(this).selected[entry.id] ?? entry.selected) }))
    this.setData({ 'cart.items': items, ...summary(items) })
  },
  toggleAll() {
    if (this.data.checkoutLoading) return
    const selected = !this.data.allSelected
    const items = this.data.cart.items.map(item => { state(this).selected[item.id] = item.available && selected; return { ...item, selected: item.available && selected } })
    this.setData({ 'cart.items': items, ...summary(items) })
  },
  touchStart(e: any) {
    if (e.target?.dataset?.control || this.data.updatingId || this.data.checkoutLoading) return
    const touch = e.touches[0]
    if (!touch) return
    const id = String(e.currentTarget.dataset.id)
    state(this).gesture = { id, x: touch.clientX, y: touch.clientY, offset: this.data.swipeId === id ? this.data.swipeOffset : 0, horizontal: false, vertical: false }
    if (this.data.swipeId !== id) this.setData({ swipeId: '', swipeOffset: 0 })
  },
  touchMove(e: any) {
    const gesture = state(this).gesture, touch = e.touches[0]
    if (!gesture || !touch || gesture.vertical) return
    const dx = touch.clientX - gesture.x, dy = touch.clientY - gesture.y
    if (!gesture.horizontal) {
      if (Math.abs(dx) < 12 && Math.abs(dy) < 12) return
      if (Math.abs(dy) >= Math.abs(dx)) { gesture.vertical = true; return }
      gesture.horizontal = true
    }
    this.setData({ swipeId: gesture.id, swipeOffset: Math.max(0, Math.min(this.data.swipeWidth, gesture.offset - dx)), dragging: true })
  },
  touchEnd() {
    const s = state(this), gesture = s.gesture
    s.gesture = undefined
    if (!gesture || (!gesture.horizontal && !gesture.vertical)) return
    s.suppressUntil = Date.now() + 400
    const open = gesture.horizontal && this.data.swipeOffset >= this.data.swipeWidth / 2
    this.setData({ swipeId: open ? gesture.id : '', swipeOffset: open ? this.data.swipeWidth : 0, dragging: false })
  },
  touchCancel() { state(this).gesture = undefined; state(this).suppressUntil = Date.now() + 400; this.setData({ swipeId: '', swipeOffset: 0, dragging: false }) },
  imageError(e: any) {
    const items = this.data.cart.items.map(item => item.id === String(e.currentTarget.dataset.id) ? { ...item, coverImageUrl: '' } : item)
    this.setData({ 'cart.items': items })
  },
  async openDetail(e: any) {
    const s = state(this)
    if (s.suppressUntil > Date.now() || s.navigating || this.data.updatingId || this.data.checkoutLoading) return
    if (this.data.swipeId) { this.setData({ swipeId: '', swipeOffset: 0 }); return }
    const item = this.data.cart.items.find(entry => entry.id === String(e.currentTarget.dataset.id))
    if (!item) return
    s.navigating = true
    try { await wx.navigateTo({ url: `/pages/services/detail?id=${encodeURIComponent(item.skuId)}` }) }
    catch { wx.showToast({ title: '服务详情打开失败，请重试', icon: 'none' }) }
    finally { s.navigating = false }
  },
  async longPress(e: any) {
    const s = state(this)
    if (e.target?.dataset?.control || this.data.updatingId || this.data.checkoutLoading || s.gesture?.horizontal || s.gesture?.vertical) return
    s.gesture = undefined; s.suppressUntil = Date.now() + 600
    const revision = s.revision
    try {
      const result = await wx.showActionSheet({ itemList: ['删除服务'] })
      if (current(this, revision) && result.tapIndex === 0) await this.remove(e)
    } catch { /* Cancelling the action sheet leaves the cart unchanged. */ }
    finally { s.suppressUntil = Date.now() + 600 }
  },
  async confirmVersion(e: any) {
    if (this.data.updatingId || this.data.checkoutLoading) return
    const item = this.data.cart.items.find(entry => entry.id === String(e.currentTarget.dataset.id))
    if (!item?.available || !item.needsConfirmation) return
    const revision = ++state(this).revision
    this.setData({ updatingId: item.id, loading: false })
    try {
      const result = await wx.showModal({ title: '确认新版服务', content: '请先查看当前服务详情，确认价格、服务说明和服务须知。确认后将采用新版内容。' })
      if (!result.confirm || !current(this, revision)) return
      const cart = await confirmCartItem(item.id, item.currentVersion)
      if (current(this, revision)) this.applyCart(cart)
    } catch (error) { if (current(this, revision)) wx.showToast({ title: error instanceof Error ? error.message : '确认失败', icon: 'none' }) }
    finally { if (current(this, revision)) this.setData({ updatingId: '' }) }
  },
  async change(e: any) {
    if (this.data.updatingId || this.data.checkoutLoading) return
    const item = this.data.cart.items.find(entry => entry.id === String(e.currentTarget.dataset.id))
    const delta = Number(e.currentTarget.dataset.delta)
    if (!item?.available || ![-1, 1].includes(delta)) return
    const quantity = item.quantity + delta
    if (!Number.isInteger(quantity) || quantity < 1 || quantity > 9999) return
    const revision = ++state(this).revision
    this.setData({ updatingId: item.id, loading: false })
    try { const cart = await updateQuantity(item.id, quantity); if (current(this, revision)) this.applyCart(cart) }
    catch (error) { if (current(this, revision)) wx.showToast({ title: error instanceof Error ? error.message : '修改失败', icon: 'none' }) }
    finally { if (current(this, revision)) this.setData({ updatingId: '' }) }
  },
  async remove(e: any) {
    if (this.data.updatingId || this.data.checkoutLoading) return
    const item = this.data.cart.items.find(entry => entry.id === String(e.currentTarget.dataset.id))
    if (!item) return
    const revision = ++state(this).revision
    this.setData({ updatingId: item.id, loading: false })
    try {
      const result = await wx.showModal({ title: '删除服务', content: `确认移除“${item.name}”？` })
      if (!result.confirm || !current(this, revision)) return
      const cart = await removeItem(item.id)
      if (current(this, revision)) { delete state(this).selected[item.id]; this.applyCart(cart) }
    } catch (error) { if (current(this, revision)) wx.showToast({ title: error instanceof Error ? error.message : '删除失败，请重试', icon: 'none' }) }
    finally { if (current(this, revision)) this.setData({ updatingId: '' }) }
  },
  goServices() { wx.switchTab({ url: '/pages/services/index' }) },
  async checkout() {
    if (this.data.checkoutLoading || this.data.updatingId) return
    const ids = this.data.cart.items.filter(item => item.available && item.selected).map(item => item.id)
    if (!ids.length) { wx.showToast({ title: '请至少选择一项服务', icon: 'none' }); return }
    const revision = ++state(this).revision
    this.setData({ checkoutLoading: true, loading: false })
    try {
      const latest = await getCart()
      if (!current(this, revision)) return
      this.applyCart(latest)
      const selected = ids.map(id => latest.items.find(item => item.id === id))
      if (selected.some(item => !item?.available)) { wx.showToast({ title: '所选服务已下架或已移除，请重新选择', icon: 'none' }); return }
      if (selected.some(item => item?.needsConfirmation)) { wx.showToast({ title: '服务信息已更新，请先确认新版', icon: 'none' }); return }
      try {
        wx.setStorageSync(checkoutItemsStorageKey, ids)
        await wx.navigateTo({ url: '/pages/checkout/index' })
      } catch (error) { wx.removeStorageSync(checkoutItemsStorageKey); throw error }
    } catch (error) { if (current(this, revision)) wx.showToast({ title: error instanceof Error ? error.message : '结算页打开失败，请重试', icon: 'none' }) }
    finally { if (current(this, revision)) this.setData({ checkoutLoading: false }) }
  },
})
