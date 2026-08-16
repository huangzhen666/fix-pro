import { getWorkerMe, logoutWorker } from '../../services/auth'
import { getApiBaseUrl } from '../../config/env'

Page({
  data: { workerName: '', workerSurname: '师', workerNo: '', mobile: '', avatarLocalUrl: '', loading: true },
  async onShow(): Promise<void> {
    try {
      const result = await getWorkerMe()
      const workerName = result.worker.displayName ?? ''
      const workerSurname = Array.from(workerName.trim())[0] || '师'
      this.setData({ workerName, workerSurname, workerNo: result.worker.workerNo ?? '', mobile: result.worker.mobile ?? '', avatarLocalUrl: '', loading: false })
      const token = wx.getStorageSync<string>('fixpro.worker.accessToken')
      const avatarUrl = result.worker.avatar?.url
      if (avatarUrl) {
        wx.downloadFile({
          url: `${getApiBaseUrl()}${avatarUrl}`,
          header: token ? { Authorization: `Bearer ${token}` } : {},
          success: response => {
            if (response.statusCode === 200) this.setData({ avatarLocalUrl: response.tempFilePath })
          },
        })
      }
    } catch { this.setData({ loading: false }) }
  },
  onChangePassword(): void { wx.navigateTo({ url: '/pages/change-password/index' }) },
  async onLogout(): Promise<void> {
    await logoutWorker()
    wx.reLaunch({ url: '/pages/login/index' })
  },
})
