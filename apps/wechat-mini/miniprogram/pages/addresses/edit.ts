import { protectedPage } from '../../services/guard'
import { createAddress, listAddresses, updateAddress, type AddressWrite } from '../../services/addresses'
import { registerPrivacyListener, finishPrivacy, queuePrivacy, type PrivacyResolve } from '../../services/privacy'
import { sessionEpoch } from '../../services/session'

const CITY = '汉川市'

protectedPage({
  data: {
    id: '',
    cityOptions: [CITY], cityIndex: 0,
    form: { city: CITY, detailAddress: '', buildingDoor: '', contactName: '', contactMobile: '', isDefault: false } as AddressWrite,
    saving: false, locating: false, showPrivacy: false, privacyTitle: '隐私保护指引',
  },
  async onLoad(options: Record<string, string>) {
    registerPrivacyListener()
    wx.getPrivacySetting?.({ success: r => this.setData({ privacyTitle: r.privacyContractName || '隐私保护指引' }) })
    const id = String(options.id || '')
    wx.setNavigationBarTitle({ title: id ? '编辑地址' : '添加地址' })
    if (!id) return
    try {
      const item = (await listAddresses()).find(address => address.id === id)
      if (!item) {
        wx.showToast({ title: '地址不存在', icon: 'none' })
        setTimeout(() => wx.navigateBack(), 500)
        return
      }
      const inServiceCity = /汉川/.test(item.city)
      this.setData({ id, form: {
        city: CITY, detailAddress: inServiceCity ? item.detailAddress : '', buildingDoor: inServiceCity ? item.buildingDoor : '',
        contactName: item.contactName, contactMobile: item.contactMobile, isDefault: item.isDefault,
      } })
      if (!inServiceCity) wx.showModal({ title: '服务城市调整', content: '目前仅提供汉川市上门服务，请重新填写汉川市的具体地址和门牌号。', showCancel: false })
    } catch (e: unknown) {
      wx.showToast({ title: e instanceof Error ? e.message : '地址加载失败', icon: 'none' })
    }
  },
  onUnload() { finishPrivacy(this, false) },
  cityChange(e: { detail: { value: string | number } }) {
    const index = Number(e.detail.value)
    const city = this.data.cityOptions[index]
    if (!city) return
    this.setData({ cityIndex: index, 'form.city': city })
  },
  input(e: WechatMiniprogram.Input) {
    const field = String(e.currentTarget.dataset.field)
    if (!['detailAddress', 'buildingDoor', 'contactName', 'contactMobile'].includes(field)) return
    this.setData({ [`form.${field}`]: e.detail.value })
  },
  clearMobile() { this.setData({ 'form.contactMobile': '' }) },
  requestPrivacy(resolve: PrivacyResolve) {
    // The pending request belongs to this page and is resolved by the native agree button.
    this.setData({ showPrivacy: true })
    queuePrivacy(this, resolve)
  },
  privacy() { wx.openPrivacyContract?.({ fail: () => wx.showToast({ title: '暂时无法打开隐私指引', icon: 'none' }) }) },
  privacyDeclined() {
    this.setData({ showPrivacy: false, locating: false })
    finishPrivacy(this, false)
  },
  privacyAgreed() {
    this.setData({ showPrivacy: false })
    finishPrivacy(this, true, 'address-privacy-agree')
  },
  stopPropagation() {},
  chooseLocation() {
    if (this.data.locating || this.data.saving) return
    if (typeof wx.chooseLocation !== 'function') {
      wx.showToast({ title: '当前微信不支持定位，请手动填写', icon: 'none' })
      return
    }
    const epoch = sessionEpoch()
    this.setData({ locating: true })
    wx.chooseLocation({
      success: result => {
        if (epoch !== sessionEpoch()) return
        const address = result.address.trim()
        if (!/汉川/.test(address)) {
          wx.showModal({ title: '请选择汉川市地址', content: '目前仅提供汉川市上门服务，请选择汉川市内的位置，或手动填写具体地址。', showCancel: false })
          return
        }
        const street = address.replace(/^.*?汉川市?/, '').trim()
        const name = result.name.trim()
        const detailAddress = name && !street.includes(name) ? `${street}${name}` : street || name
        if (detailAddress.length < 2 || detailAddress.length > 255) {
          wx.showToast({ title: '请手动补充具体地址', icon: 'none' })
          return
        }
        this.setData({ 'form.city': CITY, 'form.detailAddress': detailAddress })
      },
      fail: result => {
        if (epoch !== sessionEpoch() || /cancel/i.test(result.errMsg)) return
        if (/auth deny|auth denied|authorize.*deny/i.test(result.errMsg)) {
          wx.showModal({ title: '未开启位置权限', content: '可在微信设置中开启位置权限，也可以直接手动填写地址。', confirmText: '去设置', cancelText: '手动填写', success: r => { if (r.confirm) wx.openSetting({}) } })
        } else {
          wx.showToast({ title: '定位暂不可用，请手动填写', icon: 'none' })
        }
      },
      complete: () => { if (epoch === sessionEpoch()) this.setData({ locating: false }) },
    })
  },
  defaultChange(e: WechatMiniprogram.SwitchChange) {
    this.setData({ 'form.isDefault': Boolean(e.detail.value) })
  },
  async save() {
    if (this.data.saving || this.data.locating) return
    const form = { ...this.data.form, city: this.data.form.city.trim(), detailAddress: this.data.form.detailAddress.trim(), buildingDoor: this.data.form.buildingDoor.trim(), contactName: this.data.form.contactName.trim(), contactMobile: this.data.form.contactMobile.trim() }
    const error = !this.data.cityOptions.includes(form.city) ? '请选择所在城市' : form.detailAddress.length < 2 ? '请填写具体地址' : !form.buildingDoor ? '请填写楼号／门牌号' : form.contactName.length < 2 ? '请填写至少两字的联系人姓名' : !/^1\d{10}$/.test(form.contactMobile) ? '请填写正确的手机号码' : ''
    if (error) { wx.showToast({ title: error, icon: 'none' }); return }
    this.setData({ saving: true, form })
    try {
      if (this.data.id) await updateAddress(this.data.id, form)
      else await createAddress(form)
      wx.showToast({ title: '保存成功', icon: 'success' })
      setTimeout(() => wx.navigateBack(), 400)
    } catch (e: unknown) {
      wx.showToast({ title: e instanceof Error ? e.message : '保存失败', icon: 'none' })
    } finally {
      this.setData({ saving: false })
    }
  },
})
