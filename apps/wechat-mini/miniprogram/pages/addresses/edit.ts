import { createAddress, listAddresses, updateAddress, type AddressWrite } from '../../services/addresses'

Page({
  data: { id: '', form: { city: '', detailAddress: '', buildingDoor: '', contactName: '', contactMobile: '', isDefault: false } as AddressWrite, saving: false },
  async onLoad(options: Record<string, string>) {
    const id = String(options.id || '')
    if (!id) return
    try {
      const item = (await listAddresses()).find(address => address.id === id)
      if (!item) {
        wx.showToast({ title: '地址不存在', icon: 'none' })
        setTimeout(() => wx.navigateBack(), 500)
        return
      }
      this.setData({ id, form: { city: item.city, detailAddress: item.detailAddress, buildingDoor: item.buildingDoor, contactName: item.contactName, contactMobile: item.contactMobile, isDefault: item.isDefault } })
    } catch (e: any) {
      wx.showToast({ title: e instanceof Error ? e.message : '地址加载失败', icon: 'none' })
    }
  },
  input(e: any) {
    const field = String(e.currentTarget.dataset.field)
    this.setData({ [`form.${field}`]: e.detail.value })
  },
  chooseWechatAddress() {
    if (typeof wx.chooseAddress !== 'function') {
      wx.showToast({ title: '当前基础库不支持微信地址选择，请手工填写', icon: 'none' })
      return
    }
    wx.chooseAddress({
      success: result => {
        const city = [result.provinceName, result.cityName, result.countyName].filter(Boolean).join('')
        const mobile = /^1\d{10}$/.test(result.telNumber || '') ? result.telNumber : ''
        this.setData({
          'form.city': city,
          'form.detailAddress': result.detailInfo || '',
          'form.contactName': result.userName || this.data.form.contactName,
          'form.contactMobile': mobile || this.data.form.contactMobile,
        })
        if (!mobile && result.telNumber) wx.showToast({ title: '微信地址中的电话格式需手工确认', icon: 'none' })
      },
      fail: () => wx.showToast({ title: '未授权微信地址，请手工填写', icon: 'none' }),
    })
  },
  fillWechatUserName() {
    const apply = (userInfo?: { nickName?: string }) => {
      const nickName = String(userInfo?.nickName || '').trim()
      if (nickName && nickName !== '微信用户') this.setData({ 'form.contactName': nickName })
      else wx.showToast({ title: '未获取到有效昵称，请手工填写', icon: 'none' })
    }
    if (typeof wx.getUserProfile === 'function') {
      wx.getUserProfile({ desc: '用于填写上门服务联系人姓名', lang: 'zh_CN', success: result => apply(result.userInfo), fail: () => wx.showToast({ title: '未授权用户信息，请手工填写', icon: 'none' }) })
      return
    }
    wx.getUserInfo({ success: result => apply(result.userInfo), fail: () => wx.showToast({ title: '未授权用户信息，请手工填写', icon: 'none' }) })
  },
  defaultChange(e: any) {
    this.setData({ 'form.isDefault': Boolean(e.detail.value) })
  },
  async save() {
    if (this.data.saving) return
    const form = { ...this.data.form, city: this.data.form.city.trim(), detailAddress: this.data.form.detailAddress.trim(), buildingDoor: this.data.form.buildingDoor.trim(), contactName: this.data.form.contactName.trim(), contactMobile: this.data.form.contactMobile.trim() }
    if (form.city.length < 2 || form.detailAddress.length < 2 || !form.buildingDoor || form.contactName.length < 2 || !/^1\d{10}$/.test(form.contactMobile)) {
      wx.showToast({ title: '请完整填写地址和联系人信息', icon: 'none' })
      return
    }
    this.setData({ saving: true, form })
    try {
      if (this.data.id) await updateAddress(this.data.id, form)
      else await createAddress(form)
      wx.showToast({ title: '保存成功', icon: 'success' })
      setTimeout(() => wx.navigateBack(), 400)
    } catch (e: any) {
      wx.showToast({ title: e instanceof Error ? e.message : '保存失败', icon: 'none' })
    } finally {
      this.setData({ saving: false })
    }
  },
})
