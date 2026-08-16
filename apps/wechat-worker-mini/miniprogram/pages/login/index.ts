import { ApiError } from '../../services/request'
import { loginWorker } from '../../services/auth'

Page({
  data: { mobile: '', password: '', submitting: false, errorMessage: '' },
  onInputMobile(event: WechatMiniprogram.Input): void { this.setData({ mobile: event.detail.value, errorMessage: '' }) },
  onInputPassword(event: WechatMiniprogram.Input): void { this.setData({ password: event.detail.value, errorMessage: '' }) },
  async onLogin(): Promise<void> {
    const mobile = String(this.data.mobile).trim()
    const password = String(this.data.password)
    if (!/^1\d{10}$/.test(mobile)) {
      this.setData({ errorMessage: '请输入正确的 11 位手机号' })
      return
    }
    if (!password) {
      this.setData({ errorMessage: '请输入登录密码' })
      return
    }
    if (this.data.submitting) return
    this.setData({ submitting: true, errorMessage: '' })
    try {
      const result = await loginWorker(mobile, password)
      wx.reLaunch({ url: result.mustChangePassword ? '/pages/change-password/index' : '/pages/workbench/index' })
    } catch (error) {
      const message = error instanceof ApiError && error.code === 'WORKER_LOGIN_FAILED' ? '手机号或密码错误，请重新输入' : error instanceof Error ? error.message : '登录失败，请稍后重试'
      this.setData({ errorMessage: message })
    } finally {
      this.setData({ submitting: false })
    }
  },
})
