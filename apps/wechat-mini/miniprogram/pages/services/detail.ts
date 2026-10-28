import { getApiBaseUrl } from '../../config/env'
import { getService } from '../../services/catalog'
import { addCart, getCart } from '../../services/cart'
import { ensureReady, hasSession, openLogin, onSessionReset } from '../../services/session'

Page({
  data: { loading: true, error: '', service: null as any, quantity: 1, adding: false, ordering: false, cartCount: 0 },
  onLoad(options) { this.load(String(options.id || '')) },
  onShow() { this.refreshCart() },
  async refreshCart() {
    if (!hasSession()) { this.setData({ cartCount: 0 }); return }
    try { const cart = await getCart(); this.setData({ cartCount: cart.itemCount }) } catch { /* Keep service browsing available. */ }
  },
  async load(id: string) {
    try {
      const s = await getService(id)
      this.setData({ service: { ...s, priceText: (s.price / 100).toFixed(2), galleryImageUrls: [s.coverImageUrl, ...s.galleryImageUrls.filter(x => x !== s.coverImageUrl)].map(x => `${getApiBaseUrl()}${x}`) } })
    } catch (e) { this.setData({ error: e instanceof Error ? e.message : '加载失败' }) }
    finally { this.setData({ loading: false }) }
  },
  changeQuantity(e: WechatMiniprogram.BaseEvent) {
    if (this.data.adding || this.data.ordering) return
    const delta = Number(e.currentTarget.dataset.delta)
    this.setData({ quantity: Math.max(1, Math.min(99, this.data.quantity + delta)) })
  },
  async add() {
    if (!this.data.service || this.data.adding || this.data.ordering) return
    if (!hasSession()) { openLogin(); return }
    this.setData({ adding: true })
    try {
      const cart = await addCart(this.data.service.id, this.data.quantity)
      this.setData({ cartCount: cart.itemCount })
      wx.showToast({ title: '已加入购物车', icon: 'success' })
    } catch (e) { wx.showToast({ title: e instanceof Error ? e.message : '加购失败', icon: 'none' }) }
    finally { this.setData({ adding: false }) }
  },
  openCart() { wx.navigateTo({ url: '/pages/cart/index' }) },
  async orderNow() {
    if (!this.data.service || this.data.adding || this.data.ordering) return
    const service = this.data.service
    const url = `/pages/checkout/index?skuId=${encodeURIComponent(service.id)}&quantity=${this.data.quantity}&skuVersion=${service.publishedVersion}`
    if (!hasSession()) { openLogin(url); return }
    this.setData({ ordering: true })
    try {
      await ensureReady()
      wx.navigateTo({ url, fail: () => wx.showToast({ title: '下单页面打开失败，请重试', icon: 'none' }), complete: () => this.setData({ ordering: false }) })
    } catch (e) {
      this.setData({ ordering: false })
      wx.showToast({ title: e instanceof Error ? e.message : '请登录后下单', icon: 'none' })
    }
  },
})

onSessionReset(() => {
  for (const page of getCurrentPages()) {
    if (page.route === 'pages/services/detail') page.setData({ cartCount: 0 })
  }
})
