import { deleteAddress, listAddresses, setDefaultAddress, type CustomerAddress, formatAddress } from '../../services/addresses'

type AddressView = CustomerAddress & { fullAddress: string; maskedMobile: string }

function toView(item: CustomerAddress): AddressView {
  return { ...item, fullAddress: formatAddress(item), maskedMobile: item.contactMobile.length >= 7 ? `${item.contactMobile.slice(0, 3)}****${item.contactMobile.slice(-4)}` : item.contactMobile }
}

Page({
  data: { items: [] as AddressView[], selectMode: false, loading: false },
  onLoad(options: Record<string, string>) {
    this.setData({ selectMode: options.select === '1' })
  },
  onShow() { this.load() },
  async load() {
    this.setData({ loading: true })
    try {
      const items = (await listAddresses()).map(toView)
      this.setData({ items })
    } catch (e: any) {
      wx.showToast({ title: e instanceof Error ? e.message : '地址加载失败', icon: 'none' })
    } finally {
      this.setData({ loading: false })
    }
  },
  add() { wx.navigateTo({ url: '/pages/addresses/edit' }) },
  edit(e: any) { wx.navigateTo({ url: `/pages/addresses/edit?id=${e.currentTarget.dataset.id}` }) },
  choose(e: any) {
    if (!this.data.selectMode) return
    const item = this.data.items.find(x => x.id === String(e.currentTarget.dataset.id))
    if (!item) return
    wx.setStorageSync('fixpro.selectedAddress', item)
    wx.navigateBack()
  },
  async setDefault(e: any) {
    const id = String(e.currentTarget.dataset.id)
    if (this.data.items.find(x => x.id === id)?.isDefault) return
    try {
      await setDefaultAddress(id)
      wx.showToast({ title: '已设为默认地址', icon: 'success' })
      await this.load()
    } catch (error: any) {
      wx.showToast({ title: error instanceof Error ? error.message : '设置失败', icon: 'none' })
    }
  },
  remove(e: any) {
    const id = String(e.currentTarget.dataset.id)
    wx.showModal({
      title: '删除地址',
      content: '删除后不可恢复，确认删除吗？',
      success: async result => {
        if (!result.confirm) return
        try {
          await deleteAddress(id)
          wx.showToast({ title: '已删除', icon: 'success' })
          await this.load()
        } catch (error: any) {
          wx.showToast({ title: error instanceof Error ? error.message : '删除失败', icon: 'none' })
        }
      },
    })
  },
})
