import { showApiError } from '../../services/request'
import { protectedPage } from '../../services/guard'
import { request, upload, download } from '../../services/request'
import { updateCustomer, rememberFile, sessionEpoch, type CustomerProfile } from '../../services/session'
protectedPage({
  data: { displayName: '', mobileMasked: '', avatarPath: '', avatarMediaId: null as string | null, version: 0, changed: false, saving: false, error: '', newAvatar: '' },
  async onLoad() { await this.reload() },
  onUnload() { wx.disableAlertBeforeUnload() },
  async reload() {
    wx.disableAlertBeforeUnload()
    const p = await request<CustomerProfile>({ url: '/api/v1/mini/profile' })
    this.setData({ displayName: p.displayName, mobileMasked: p.mobileMasked, avatarMediaId: p.avatarMediaId, version: p.version, changed: false, newAvatar: '', avatarPath: '' })
    if (p.avatarUrl) { try { this.setData({ avatarPath: await download(p.avatarUrl) }) } catch { /* placeholder remains */ } }
  },
  nickname(e: any) { wx.enableAlertBeforeUnload({ message: "未保存的资料将被丢弃" }); this.setData({ displayName: e.detail.value, changed: true }) },
  chooseAvatar(e: any) { if (e.detail.avatarUrl) rememberFile(e.detail.avatarUrl); if (e.detail.avatarUrl) wx.enableAlertBeforeUnload({ message: "未保存的资料将被丢弃" }); if (e.detail.avatarUrl) this.setData({ avatarPath: e.detail.avatarUrl, newAvatar: e.detail.avatarUrl, changed: true }) },
  cancel() {
    if (this.data.saving) return
    if (!this.data.changed) { wx.navigateBack(); return }
    wx.showModal({ title: '放弃修改？', content: '未保存的头像和昵称将被丢弃。', success: r => { if (r.confirm) { wx.disableAlertBeforeUnload(); wx.navigateBack() } } })
  },
  async save() {
    if (this.data.saving) return
    const name = this.data.displayName.trim()
    if (!name || [...name].length > 32) { this.setData({ error: '昵称需为 1 至 32 个字符' }); return }
    const epoch = sessionEpoch()
    this.setData({ saving: true, error: '' })
    try {
      let avatarMediaId = this.data.avatarMediaId
      if (this.data.newAvatar) {
        const uploaded = await upload<{ mediaId: string }>('/api/v1/mini/media/avatar', this.data.newAvatar)
        avatarMediaId = uploaded.mediaId
        this.setData({ avatarMediaId, newAvatar: '' })
      }
      const p = await request<CustomerProfile>({ url: '/api/v1/mini/profile', method: 'PUT', data: { displayName: name, avatarMediaId, version: this.data.version } })
      if (epoch !== sessionEpoch()) return
      updateCustomer(p); wx.disableAlertBeforeUnload(); this.setData({ changed: false }); wx.showToast({ title: "保存成功" }); wx.navigateBack()
    } catch (e: any) {
 showApiError(e)
      if (epoch !== sessionEpoch()) return
      this.setData({ error: e instanceof Error ? e.message : '保存失败，请重试' })
      if (e?.code === 'RESOURCE_VERSION_CONFLICT') wx.showModal({ title: '资料已更新', content: '重新载入最新资料后编辑？当前草稿会被丢弃。', success: r => { if (r.confirm) this.reload().catch(() => {}) } })
    } finally { if (epoch === sessionEpoch()) this.setData({ saving: false }) }
  },
})
