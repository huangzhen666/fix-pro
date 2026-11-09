import { sessionEpoch } from '../../services/session'
import { uploadDemand } from '../../services/cart'
import { download } from '../../services/request'
import { type DemandDraftItem } from '../../services/demand'

Component({
  properties: {
    open: { type: Boolean, value: false },
    items: { type: Array, value: [], observer(value: DemandDraftItem[]) { this.setData({ draft: JSON.parse(JSON.stringify(value || [])) }); if (!value?.length) this.setData({ pendingPath: '', pendingId: '', uploadError: '' }); void this.loadThumbnails() } },
    saving: { type: Boolean, value: false },
    error: { type: String, value: '' },
    conflict: { type: Boolean, value: false },
    previousItems: { type: Array, value: [] },
  },
  data: { draft: [] as DemandDraftItem[], uploading: false, uploadError: '', pendingPath: '', pendingId: '', alive: true, keyboardHeight: 0 },
  lifetimes: { detached() { this.data.alive = false } },
  methods: {
    noop() {},
    keyboard(e: WechatMiniprogram.CustomEvent<{ height: number }>) { this.setData({ keyboardHeight: e.detail.height || 0 }) },
    blur() { this.setData({ keyboardHeight: 0 }) },
    restore() { this.setData({ draft: JSON.parse(JSON.stringify(this.data.previousItems)) }); this.notify(); void this.loadThumbnails() },
    async loadThumbnails() {
      const tasks = this.data.draft.flatMap(item => item.demandMedia.filter(media => media.mediaType === 'IMAGE' && !media.localUrl).map(async media => {
        try {
          const localUrl = await download(`/api/v1/mini/media/${media.id}/content`)
          if (!this.data.alive) return
          const i = this.data.draft.findIndex(row => row.itemId === item.itemId)
          const m = this.data.draft[i]?.demandMedia.findIndex(row => row.id === media.id) ?? -1
          if (i >= 0 && m >= 0) this.setData({ [`draft[${i}].demandMedia[${m}].localUrl`]: localUrl })
        } catch { /* A readable placeholder remains available for a preview retry. */ }
      }))
      await Promise.allSettled(tasks)
    },
    notify() { this.triggerEvent('change', { items: this.data.draft }) },
    close() { if (!this.data.saving && !this.data.uploading) this.triggerEvent('close') },
    input(e: WechatMiniprogram.Input) {
      const id = String(e.currentTarget.dataset.id)
      const index = this.data.draft.findIndex(item => item.itemId === id)
      if (index < 0) return
      this.setData({ [`draft[${index}].demandDescription`]: e.detail.value }); this.notify()
    },
    async choose(e: WechatMiniprogram.BaseEvent) {
      if (this.data.uploading || this.data.saving) return
      const id = String(e.currentTarget.dataset.id)
      const epoch = sessionEpoch()
      this.setData({ uploading: true }); this.triggerEvent('busy', { busy: true })
      try {
        const chosen = await wx.chooseMedia({ count: 1, mediaType: ['image', 'video'] })
        if (!this.data.alive || epoch !== sessionEpoch()) return
        if (chosen.tempFiles.length) {
          this.setData({ pendingPath: chosen.tempFiles[0].tempFilePath, pendingId: id })
          await this.uploadFile()
        }
      } catch (e) {
        if (!String((e as { errMsg?: string }).errMsg || '').includes('cancel') && this.data.alive) this.setData({ uploadError: e instanceof Error ? e.message : '资料选择失败，请重试' })
      } finally { if (this.data.alive) { this.setData({ uploading: false }); this.triggerEvent('busy', { busy: false }) } }
    },
    async uploadFile() {
      try {
        const path = this.data.pendingPath, id = this.data.pendingId
        const result = await uploadDemand(path)
        if (!this.data.alive) return
        const index = this.data.draft.findIndex(item => item.itemId === id)
        if (index < 0) return
        const item = this.data.draft[index]
        this.setData({ [`draft[${index}].demandMedia`]: [...item.demandMedia, { id: String(result.id), mediaType: result.mediaType, name: '现场资料', url: `/api/v1/mini/media/${result.id}/content`, localUrl: path }], pendingPath: '', pendingId: '', uploadError: '' })
        this.notify()
      } catch (e) { if (this.data.alive) this.setData({ uploadError: e instanceof Error ? e.message : '上传失败，请重试' }) }
    },
    async retry() {
      if (!this.data.pendingPath || this.data.uploading || this.data.saving) return
      this.setData({ uploading: true }); this.triggerEvent('busy', { busy: true })
      try { await this.uploadFile() } finally { if (this.data.alive) { this.setData({ uploading: false }); this.triggerEvent('busy', { busy: false }) } }
    },
    ignore() { this.setData({ pendingPath: '', pendingId: '', uploadError: '' }) },
    remove(e: WechatMiniprogram.BaseEvent) {
      if (this.data.uploading || this.data.saving) return
      const index = this.data.draft.findIndex(item => item.itemId === String(e.currentTarget.dataset.id))
      if (index < 0) return
      this.setData({ [`draft[${index}].demandMedia`]: this.data.draft[index].demandMedia.filter(media => media.id !== String(e.currentTarget.dataset.media)) }); this.notify()
    },
    async preview(e: WechatMiniprogram.BaseEvent) {
      const item = this.data.draft.find(item => item.itemId === String(e.currentTarget.dataset.id))
      const media = item?.demandMedia.find(media => media.id === String(e.currentTarget.dataset.media))
      if (!media) return
      try {
        const url = media.localUrl || await download(`/api/v1/mini/media/${media.id}/content`)
        if (!this.data.alive) return
        if (media.mediaType === 'VIDEO') wx.previewMedia({ sources: [{ url, type: 'video' }] })
        else wx.previewImage({ current: url, urls: [url] })
      } catch { wx.showToast({ title: '资料暂时无法预览，请重试', icon: 'none' }) }
    },
    imageError(e: WechatMiniprogram.BaseEvent) {
      const index = this.data.draft.findIndex(item => item.itemId === String(e.currentTarget.dataset.id))
      const m = this.data.draft[index]?.demandMedia.findIndex(media => media.id === String(e.currentTarget.dataset.media)) ?? -1
      if (index >= 0 && m >= 0) this.setData({ [`draft[${index}].demandMedia[${m}].localUrl`]: '' })
    },
    save() { if (!this.data.uploading && !this.data.saving) this.triggerEvent('save', { items: this.data.draft }) },
    refresh() { if (!this.data.uploading && !this.data.saving) this.triggerEvent('refresh') },
  },
})
