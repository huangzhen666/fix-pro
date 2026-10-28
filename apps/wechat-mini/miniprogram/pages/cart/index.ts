import { protectedPage } from '../../services/guard'
import { getApiBaseUrl } from '../../config/env'
import { getCart, removeItem, saveFault, updateQuantity, uploadFault, type Cart, type CartItem } from '../../services/cart'
import { download } from '../../services/request'
import { onSessionReset } from '../../services/session'

const checkoutItemsStorageKey = 'fixpro.checkoutCartItemIds'
type DisplayCartItem = CartItem & { coverImageUrl: string; subtotalText: string; selected: boolean }
type DisplayCart = Omit<Cart, 'items'> & { items: DisplayCartItem[] }
let selectedByItemID: Record<string, boolean> = {}
type Gesture = { id: string; x: number; y: number; offset: number; horizontal: boolean; vertical: boolean }
const gestures = new WeakMap<object, Gesture>()
onSessionReset(() => { selectedByItemID = {} })

function summary(items: DisplayCartItem[]) {
  const available = items.filter(item => item.available)
  const selected = available.filter(item => item.selected)
  return { totalText: (selected.reduce((sum, item) => sum + item.subtotal, 0) / 100).toFixed(2), selectedCount: selected.length, allSelected: !!available.length && selected.length === available.length }
}

protectedPage({
  data: {
    loading: true, cart: { items: [], itemCount: 0, totalAmount: 0 } as DisplayCart,
    totalText: '0.00', selectedCount: 0, allSelected: false, savingId: '', updatingId: '', checkoutLoading: false,
    expandedId: '', swipeId: '', swipeOffset: 0, swipeWidth: 90, dragging: false,
  },
  onLoad() { this.setData({ swipeWidth: wx.getWindowInfo().windowWidth * 144 / 750 }) },
  onShow() { this.load() },
  onPullDownRefresh() { this.load().finally(() => wx.stopPullDownRefresh()) },
  onUnload() { selectedByItemID = {}; gestures.delete(this) },
  applyCart(cart: Cart) {
    const previous = new Map(this.data.cart.items.map(item => [item.id, item]))
    const items = cart.items.map(item => ({ ...item, coverImageUrl: `${getApiBaseUrl()}${item.coverImageUrl}`, subtotalText: (item.subtotal / 100).toFixed(2), selected: item.available && (selectedByItemID[item.id] ?? true),
      faultMedia: item.faultMedia.map(media => ({ ...media, localUrl: (previous.get(item.id)?.faultMedia.find(old => old.id === media.id) as any)?.localUrl || '' })),
    }))
    this.setData({ cart: { ...cart, items }, ...summary(items), swipeId: '', swipeOffset: 0, dragging: false })
    this.loadPrivateMedia(items)
  },
  async load() {
    this.setData({ loading: true })
    try { this.applyCart(await getCart()) }
    catch (e) { wx.showToast({ title: e instanceof Error ? e.message : '购物车加载失败', icon: 'none' }) }
    finally { this.setData({ loading: false }) }
  },
  loadPrivateMedia(items: CartItem[]) {
    items.forEach(item => item.faultMedia.forEach(async media => {
      if ((media as any).localUrl || !media.url) return
      try {
        const localUrl = await download(media.url)
        const index = this.data.cart.items.findIndex(entry => entry.id === item.id)
        const mediaIndex = this.data.cart.items[index]?.faultMedia.findIndex(entry => entry.id === media.id) ?? -1
        if (index >= 0 && mediaIndex >= 0) this.setData({ [`cart.items[${index}].faultMedia[${mediaIndex}].localUrl`]: localUrl })
      } catch { /* Keep the service card available if media cannot be loaded. */ }
    }))
  },
  toggleSelected(e: any) {
    const index = Number(e.currentTarget.dataset.index)
    const item = this.data.cart.items[index]
    if (!item || !item.available || this.data.checkoutLoading) return
    selectedByItemID[item.id] = !item.selected
    const items = this.data.cart.items.map(entry => ({ ...entry, selected: selectedByItemID[entry.id] ?? entry.selected }))
    this.setData({ 'cart.items': items, ...summary(items) })
  },
  toggleAll() {
    if (this.data.checkoutLoading) return
    const selected = !this.data.allSelected
    const items = this.data.cart.items.map(item => { selectedByItemID[item.id] = item.available && selected; return { ...item, selected: item.available && selected } })
    this.setData({ 'cart.items': items, ...summary(items) })
  },
  touchStart(e: any) {
    if (this.data.updatingId || this.data.checkoutLoading) return
    const touch = e.touches[0]
    if (!touch) return
    const id = String(e.currentTarget.dataset.id)
    gestures.set(this, { id, x: touch.clientX, y: touch.clientY, offset: this.data.swipeId === id ? this.data.swipeOffset : 0, horizontal: false, vertical: false })
    if (this.data.swipeId !== id) this.setData({ swipeId: '', swipeOffset: 0 })
  },
  touchMove(e: any) {
    const gesture = gestures.get(this), touch = e.touches[0]
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
    const gesture = gestures.get(this)
    gestures.delete(this)
    if (!gesture?.horizontal) return
    const open = this.data.swipeOffset >= this.data.swipeWidth / 2
    this.setData({ swipeId: open ? gesture.id : '', swipeOffset: open ? this.data.swipeWidth : 0, dragging: false })
  },
  touchCancel() { gestures.delete(this); this.setData({ swipeId: '', swipeOffset: 0, dragging: false }) },
  toggleDetails(e: any) {
    const id = String(e.currentTarget.dataset.id)
    this.setData({ expandedId: this.data.expandedId === id ? '' : id, swipeId: '', swipeOffset: 0 })
  },
  inputFault(e: any) { this.setData({ [`cart.items[${Number(e.currentTarget.dataset.index)}].faultDescription`]: e.detail.value }) },
  async blurFault(e: any) { await this.persist(Number(e.currentTarget.dataset.index)) },
  async change(e: any) {
    if (this.data.updatingId || this.data.savingId || this.data.checkoutLoading) return
    const item = this.data.cart.items[Number(e.currentTarget.dataset.index)]
    if (!item) return
    if (!item.available) return
    const quantity = item.quantity + Number(e.currentTarget.dataset.delta)
    if (quantity < 1 || quantity > 99) return
    this.setData({ updatingId: item.id })
    try { this.applyCart(await updateQuantity(item.id, quantity)) }
    catch (err) { wx.showToast({ title: err instanceof Error ? err.message : '修改失败', icon: 'none' }) }
    finally { this.setData({ updatingId: '' }) }
  },
  async choose(e: any) {
    if (this.data.savingId || this.data.updatingId || this.data.checkoutLoading) return
    const index = Number(e.currentTarget.dataset.index), item = this.data.cart.items[index]
    if (!item) return
    try {
      const chosen = await wx.chooseMedia({ count: 1, mediaType: ['image', 'video'], sourceType: ['album', 'camera'] })
      if (!chosen.tempFiles.length) return
      wx.showLoading({ title: '上传中' })
      const uploaded = await uploadFault(chosen.tempFiles[0].tempFilePath)
      const faultMedia = [...item.faultMedia, { id: String(uploaded.id), mediaType: uploaded.mediaType, name: '新上传文件', url: '', localUrl: chosen.tempFiles[0].tempFilePath }]
      this.setData({ [`cart.items[${index}].faultMedia`]: faultMedia })
      await this.persist(index)
    } catch (err) { wx.showToast({ title: err instanceof Error ? err.message : '上传失败', icon: 'none' }) }
    finally { wx.hideLoading() }
  },
  async removeMedia(e: any) {
    if (this.data.savingId || this.data.updatingId || this.data.checkoutLoading) return
    const index = Number(e.currentTarget.dataset.index), mediaIndex = Number(e.currentTarget.dataset.mediaIndex)
    const item = this.data.cart.items[index]
    if (!item) return
    const list = [...item.faultMedia]; list.splice(mediaIndex, 1)
    this.setData({ [`cart.items[${index}].faultMedia`]: list }); await this.persist(index)
  },
  async persist(index: number): Promise<boolean> {
    const item = this.data.cart.items[index]
    if (!item) return true
    this.setData({ savingId: item.id })
    try { await saveFault(item.id, item.faultDescription || '', item.faultMedia.map(media => media.id)); return true }
    catch (err) { wx.showToast({ title: err instanceof Error ? err.message : '补充信息同步失败', icon: 'none' }); return false }
    finally { this.setData({ savingId: '' }) }
  },
  async remove(e: any) {
    if (this.data.updatingId || this.data.savingId || this.data.checkoutLoading) return
    const item = this.data.cart.items[Number(e.currentTarget.dataset.index)]
    if (!item) return
    this.setData({ updatingId: item.id })
    try {
      const result = await wx.showModal({ title: '删除服务', content: `确认移除“${item.name}”？` })
      if (!result.confirm) return
      const cart = await removeItem(item.id)
      delete selectedByItemID[item.id]
      this.applyCart(cart)
    } catch (err) { wx.showToast({ title: err instanceof Error ? err.message : '删除失败，请重试', icon: 'none' }) }
    finally { this.setData({ updatingId: '' }) }
  },
  goServices() { wx.switchTab({ url: '/pages/services/index' }) },
  async checkout() {
    if (this.data.checkoutLoading || this.data.savingId || this.data.updatingId) return
    const selectedItems = this.data.cart.items.filter(item => item.available && item.selected)
    if (!selectedItems.length) { wx.showToast({ title: '请至少选择一项服务', icon: 'none' }); return }
    this.setData({ checkoutLoading: true })
    // Save drafts before leaving; optional supplement fields must not be lost.
    for (const item of selectedItems) {
      if (!await this.persist(this.data.cart.items.findIndex(entry => entry.id === item.id))) { this.setData({ checkoutLoading: false }); return }
    }
    try {
      const latest = await getCart()
      this.applyCart(latest)
      if (selectedItems.some(item => !latest.items.find(current => current.id === item.id)?.available)) {
        wx.showToast({ title: '所选服务已下架或已移除，请重新选择', icon: 'none' })
        this.setData({ checkoutLoading: false })
        return
      }
    } catch (err) {
      wx.showToast({ title: err instanceof Error ? err.message : '购物车加载失败', icon: 'none' })
      this.setData({ checkoutLoading: false })
      return
    }
    wx.setStorageSync(checkoutItemsStorageKey, selectedItems.map(item => item.id))
    wx.navigateTo({ url: '/pages/checkout/index', fail: () => {
      wx.removeStorageSync(checkoutItemsStorageKey)
      wx.showToast({ title: '结算页打开失败，请重试', icon: 'none' })
    }, complete: () => this.setData({ checkoutLoading: false }) })
  },
})
