import { listCategoryGroups, type ServiceSummary } from '../../services/catalog'
import { getCart } from '../../services/cart'
import { categoryIcon, categoryLabel, mediaUrl } from '../../services/presentation'
import { hasSession, onSessionReset, sessionEpoch } from '../../services/session'

type RecommendedService = ServiceSummary & { priceText: string; imageFailed: boolean }
const catalogRequests = new WeakMap<object, number>()
const cartRequests = new WeakMap<object, number>()

Page({
  data: {
    loading: true, error: '', categories: [] as Array<{ id: string; name: string; displayName: string; iconUrl: string }>,
    recommended: [] as RecommendedService[], cartCount: 0,
  },
  onLoad() { wx.setNavigationBarTitle({ title: '首页' }) },
  onShow() { void this.load(); void this.refreshCart() },
  onPullDownRefresh() { Promise.allSettled([this.load(), this.refreshCart()]).finally(() => wx.stopPullDownRefresh()) },
  onUnload() { catalogRequests.delete(this); cartRequests.delete(this) },
  async load() {
    const request = (catalogRequests.get(this) || 0) + 1
    catalogRequests.set(this, request)
    this.setData({ loading: true, error: '' })
    try {
      const groups = await listCategoryGroups()
      if (catalogRequests.get(this) !== request) return
      const categories = groups.slice(0, 8).map(group => ({ id: group.id, name: group.name, displayName: categoryLabel(group.name), iconUrl: categoryIcon(group.id, group.name) }))
      const seen = new Set<string>()
      const recommended = groups.flatMap(group => group.services).filter(service => {
        if (seen.has(service.id)) return false
        seen.add(service.id); return true
      }).slice(0, 4).map(service => ({ ...service, coverImageUrl: mediaUrl(service.coverImageUrl), priceText: (service.price / 100).toFixed(2), imageFailed: !service.coverImageUrl }))
      this.setData({ categories, recommended })
    } catch {
      if (catalogRequests.get(this) === request) this.setData({ error: '服务加载失败，请稍后重试' })
    } finally {
      if (catalogRequests.get(this) === request) this.setData({ loading: false })
    }
  },
  async refreshCart() {
    const request = (cartRequests.get(this) || 0) + 1
    cartRequests.set(this, request)
    const epoch = sessionEpoch()
    if (!hasSession()) { this.setData({ cartCount: 0 }); return }
    try {
      const cart = await getCart()
      if (cartRequests.get(this) === request && epoch === sessionEpoch()) this.setData({ cartCount: cart.itemCount })
    } catch {
      if (cartRequests.get(this) === request && epoch === sessionEpoch()) this.setData({ cartCount: 0 })
    }
  },
  imageError(e: WechatMiniprogram.BaseEvent) {
    const id = String(e.currentTarget.dataset.id)
    const index = this.data.recommended.findIndex(service => service.id === id)
    if (index >= 0) this.setData({ [`recommended[${index}].imageFailed`]: true })
  },
  openSearch() { wx.navigateTo({ url: '/pages/search/index' }) },
  openCategory(e: WechatMiniprogram.BaseEvent) {
    wx.setStorageSync('fixpro.selectedCategory', String(e.currentTarget.dataset.id || ''))
    wx.switchTab({ url: '/pages/services/index' })
  },
  openService(e: WechatMiniprogram.BaseEvent) { wx.navigateTo({ url: `/pages/services/detail?id=${encodeURIComponent(String(e.currentTarget.dataset.id))}` }) },
  openCart() { wx.navigateTo({ url: '/pages/cart/index' }) },

})

onSessionReset(() => {
  for (const page of getCurrentPages()) {
    if (page.route !== 'pages/home/index') continue
    cartRequests.set(page, (cartRequests.get(page) || 0) + 1)
    page.setData({ cartCount: 0 })
  }
})
