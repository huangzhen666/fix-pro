import { getCart } from '../../services/cart'
import { listAddresses, type CustomerAddress } from '../../services/addresses'

Page({
  data: { cartCount: 0, defaultAddress: null as CustomerAddress | null },
  onShow() {
    Promise.all([getCart(), listAddresses()]).then(([cart, addresses]) => this.setData({ cartCount: cart.itemCount, defaultAddress: addresses.find(item => item.isDefault) || addresses[0] || null })).catch(() => {})
  },
  openCart() { wx.navigateTo({ url: '/pages/cart/index' }) },
  openOrders() { wx.navigateTo({ url: '/pages/orders/index' }) },
  openAddresses() { wx.navigateTo({ url: '/pages/addresses/index' }) },
  placeholder(e: any) { wx.showToast({ title: `${e.currentTarget.dataset.name}将在后续版本开放`, icon: 'none' }) },
  contact() { wx.showModal({ title: '联系客服', content: '本地演示环境暂未配置企业微信或客服电话。', showCancel: false }) },
})
