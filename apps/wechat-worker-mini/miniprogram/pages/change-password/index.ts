import { ApiError, clearWorkerSession } from '../../services/request'
import { changeWorkerPassword } from '../../services/auth'

Page({
  data: { currentPassword: '', newPassword: '', confirmPassword: '', submitting: false, errorMessage: '' },
  onInput(event: WechatMiniprogram.Input): void {
    const field = event.currentTarget.dataset.field as 'currentPassword' | 'newPassword' | 'confirmPassword'
    this.setData({ [field]: event.detail.value, errorMessage: '' })
  },
  async onSubmit(): Promise<void> {
    const { currentPassword, newPassword, confirmPassword } = this.data
    if (newPassword !== confirmPassword) { this.setData({ errorMessage: '两次输入的新密码不一致' }); return }
    if (newPassword.length < 12 || !/[A-Za-z]/.test(newPassword) || !/\d/.test(newPassword)) { this.setData({ errorMessage: '新密码至少 12 位，且必须包含字母和数字' }); return }
    if (this.data.submitting) return
    this.setData({ submitting: true, errorMessage: '' })
    try {
      await changeWorkerPassword(currentPassword, newPassword, confirmPassword)
      clearWorkerSession()
      wx.showToast({ title: '密码修改成功，请重新登录', icon: 'none', duration: 1800 })
      setTimeout(() => wx.reLaunch({ url: '/pages/login/index' }), 1800)
    } catch (error) {
      const message = error instanceof ApiError && error.code === 'WORKER_CURRENT_PASSWORD_INVALID' ? '当前密码错误，请重新输入' : error instanceof Error ? error.message : '密码修改失败，请稍后重试'
      this.setData({ errorMessage: message })
    } finally { this.setData({ submitting: false }) }
  },
})
