import { bindEvidence, commandWorkOrder, getWorkOrder, submitCompletion, uploadEvidence, workerReschedule, workerReturn, type Evidence, type WorkOrder } from '../../services/work-orders'
import { workOrderStatusLabel } from '../../services/status'
import { getApiBaseUrl } from '../../config/env'

const weekNames = ['日', '一', '二', '三', '四', '五', '六']

function pad(value: number) { return String(value).padStart(2, '0') }

function formatAppointmentDate(value?: string) {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return `${date.getFullYear()}年${pad(date.getMonth() + 1)}月${pad(date.getDate())}日（周${weekNames[date.getDay()]}）`
}

Page({
  data: { loading: true, submitting: false, error: '', order: null as WorkOrder | null, orderId: '', summary: '', beforeMediaId: '', afterMediaId: '' },
  onLoad(query: { id?: string }) { if (query.id) this.setData({ orderId: query.id }, () => this.load()) },
  async load() {
    try { this.setData({ loading: true, error: '' }); const order = await getWorkOrder(this.data.orderId); const view = { ...order, statusText: workOrderStatusLabel(order.status), appointmentDateText: formatAppointmentDate(order.appointmentAt) }; this.setData({ order: view }); this.loadCustomerMedia(view.items ?? []) }
    catch (error) { this.setData({ error: error instanceof Error ? error.message : '工单加载失败' }) }
    finally { this.setData({ loading: false }) }
  },
  loadCustomerMedia(items: NonNullable<WorkOrder['items']>) {
    const token = wx.getStorageSync<string>('fixpro.worker.accessToken')
    items.forEach((item, itemIndex) => item.customerMedia.forEach((media, mediaIndex) => wx.downloadFile({
      url: `${getApiBaseUrl()}${media.url}`,
      header: token ? { Authorization: `Bearer ${token}` } : {},
      success: result => {
        if (result.statusCode === 200) this.setData({ [`order.items[${itemIndex}].customerMedia[${mediaIndex}].localUrl`]: result.tempFilePath })
      },
    })))
  },
  callCustomer() {
    const mobile = this.data.order?.customerMobile
    if (!mobile) { wx.showToast({ title: '客户电话暂未提供', icon: 'none' }); return }
    wx.makePhoneCall({ phoneNumber: mobile })
  },
  async command(e: WechatMiniprogram.TouchEvent) {
    const command = (e.currentTarget as any).dataset.command as 'ACCEPT' | 'REJECT' | 'ARRIVE' | 'START'
    if (!this.data.order) return
    const commandText: Record<typeof command, string> = {
      ACCEPT: '接单',
      REJECT: '拒绝接单',
      ARRIVE: '标记已到达',
      START: '开始服务',
    }
    let reason = ''
    if (command === 'REJECT') {
      const result = await new Promise<WechatMiniprogram.ShowModalSuccessCallbackResult>(resolve => wx.showModal({ title: '拒绝接单', editable: true, placeholderText: '请说明暂时无法接单的原因', success: resolve }))
      if (!result.confirm || !result.content?.trim()) return
      reason = result.content.trim()
    } else {
      const content = command === 'ACCEPT'
        ? '确认接下这个工单吗？接单后请按客户预约时间提供服务。'
        : command === 'ARRIVE'
          ? '确认已经到达客户服务地址吗？'
          : '确认现在开始为客户提供服务吗？'
      const result = await new Promise<WechatMiniprogram.ShowModalSuccessCallbackResult>(resolve => wx.showModal({ title: `确认${commandText[command]}`, content, success: resolve }))
      if (!result.confirm) return
    }
    try { await commandWorkOrder(this.data.order.id, command, this.data.order.version, reason); wx.showToast({ title: `${commandText[command]}成功`, icon: 'success' }); await this.load() }
    catch (error) { wx.showToast({ title: error instanceof Error ? error.message : '操作失败', icon: 'none' }) }
  },
  async reschedule() { if (!this.data.order) return; const first = await new Promise<WechatMiniprogram.ShowModalSuccessCallbackResult>(resolve => wx.showModal({ title: '发起改期', editable: true, placeholderText: '请输入 YYYY-MM-DD HH:MM，例如 2026-08-12 10:00', success: resolve })); if (!first.confirm || !first.content?.trim()) return; const value = first.content.trim(); const match = value.match(/^(\d{4}-\d{2}-\d{2})\s(08|10|12|14|16|18|20):00$/); if (!match) { wx.showToast({ title: '时间必须是两小时预约段', icon: 'none' }); return } const confirmed = await new Promise<WechatMiniprogram.ShowModalSuccessCallbackResult>(resolve => wx.showModal({ title: '确认发起改期', content: `请确认已与客户沟通，并改为 ${value}。提交后由后台留痕。`, success: resolve })); if (!confirmed.confirm) return; try { await workerReschedule(this.data.order.id, `${match[1]}T${match[2]}:00:00`, `${match[2]}:00`, this.data.order.version); wx.showToast({ title: '改期已提交', icon: 'success' }); await this.load() } catch (e) { wx.showToast({ title: e instanceof Error ? e.message : '改期失败', icon: 'none' }) } },
  async returnWorkOrder() { if (!this.data.order) return; const result = await new Promise<WechatMiniprogram.ShowModalSuccessCallbackResult>(resolve => wx.showModal({ title: '退回待重新派单', editable: true, placeholderText: '请输入退回原因', success: resolve })); if (!result.confirm || !result.content?.trim()) return; const confirm = await new Promise<WechatMiniprogram.ShowModalSuccessCallbackResult>(resolve => wx.showModal({ title: '确认退回工单？', content: '退回后由履约调度员重新派单。', success: resolve })); if (!confirm.confirm) return; try { await workerReturn(this.data.order.id, this.data.order.version, result.content.trim()); wx.showToast({ title: '已退回调度', icon: 'success' }); await this.load() } catch (e) { wx.showToast({ title: e instanceof Error ? e.message : '退回失败', icon: 'none' }) } },
  chooseMedia(e: WechatMiniprogram.TouchEvent) {
    const stage = (e.currentTarget as any).dataset.stage as 'BEFORE' | 'AFTER'
    wx.chooseMedia({ count: 1, mediaType: ['image', 'video'], sourceType: ['album', 'camera'], success: async result => {
      const file = result.tempFiles[0]
      if (!file) return
      try {
        const uploaded = await uploadEvidence(this.data.orderId, file.tempFilePath)
        await bindEvidence(this.data.orderId, uploaded.id, stage, this.data.order?.version ?? 0)
        wx.showToast({ title: '凭证已上传', icon: 'success' }); await this.load()
      } catch (error) { wx.showToast({ title: error instanceof Error ? error.message : '上传失败', icon: 'none' }) }
    } })
  },
  updateSummary(e: WechatMiniprogram.Input) { this.setData({ summary: e.detail.value }) },
  async submit() {
    if (!this.data.order || this.data.summary.trim().length < 5) { wx.showToast({ title: '请输入至少 5 字完工说明', icon: 'none' }); return }
    try { this.setData({ submitting: true }); await submitCompletion(this.data.order.id, this.data.summary, this.data.order.version); wx.showToast({ title: '已提交完工', icon: 'success' }); await this.load() }
    catch (error) { wx.showToast({ title: error instanceof Error ? error.message : '提交失败', icon: 'none' }) }
    finally { this.setData({ submitting: false }) }
  },
  hasStage(stage: Evidence['stage']): boolean { return Boolean(this.data.order?.evidence?.some(item => item.stage === stage)) },
})
