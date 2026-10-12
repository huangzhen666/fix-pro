import { showApiError } from '../../services/request'
import { phoneLogin, hasSession } from '../../services/session'
import { registerPrivacyListener, finishPrivacy, queuePrivacy, type PrivacyResolve } from '../../services/privacy'
Page({
  data: { busy: false, error: '', returnUrl: '/pages/profile/index', showPrivacy: false, privacyTitle: '隐私保护指引' },
  onLoad(options) {
    let target = '/pages/profile/index'
    try { if (options.returnUrl) target = decodeURIComponent(options.returnUrl) } catch { /* keep safe default */ }
    if (/^\/pages\/[a-z/-]+(?:\?[^#]*)?$/.test(target) && !target.startsWith('/pages/login/')) this.setData({ returnUrl: target })
    registerPrivacyListener()
    wx.getPrivacySetting?.({ success: r => this.setData({ privacyTitle: r.privacyContractName || '隐私保护指引' }) })
  },
  browse() { wx.switchTab({ url: '/pages/home/index' }) },
  onUnload() { finishPrivacy(this, false) },
  requestPrivacy(resolve: PrivacyResolve) {
    queuePrivacy(this, resolve)
    this.setData({ showPrivacy: true, error: '' })
  },
  stopPropagation() { /* keep taps inside the privacy dialog */ },
  privacyDeclined() {
    this.setData({ showPrivacy: false, error: '' })
    finishPrivacy(this, false)
  },
  privacy() { wx.openPrivacyContract?.({ fail: () => wx.showToast({ title: '请在微信后台配置隐私指引', icon: 'none' }) }) },
  privacyAgreed() {
    this.setData({ showPrivacy: false, error: '' })
    finishPrivacy(this, true)
  },
  terms() { wx.showModal({ title: '服务协议', content: '手机号用于注册登录和维修服务联系。下单前请确认服务范围、报价和质保约定；订单记录保留服务信息。若有疑问请通过页面客服入口联系平台。', showCancel: false }) },
  async login(e: any) {
    if (this.data.busy) return
    if (!e.detail.code) {
      const cancelled = /deny|cancel|privacy permission/i.test(e.detail.errMsg || '')
      this.setData({ error: cancelled ? '' : '未取得手机号授权，请重试' })
      return
    }
    this.setData({ busy: true, error: '' })
    try { await phoneLogin(e.detail.code); if (!hasSession()) return; const target = this.data.returnUrl; const tabs = ['/pages/home/index', '/pages/services/index', '/pages/profile/index']; if (tabs.includes(target.split('?')[0])) wx.switchTab({ url: target.split('?')[0] }); else wx.redirectTo({ url: target }) }
    catch (e) { this.setData({ error: e instanceof Error ? e.message : '登录失败，请重试' }); showApiError(e) }
    finally { this.setData({ busy: false }) }
  },
})
