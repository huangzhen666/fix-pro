import { onSessionReset } from '../../services/session'
import { getOrderDemand, updateOrderDemand, type OrderDemand, type DemandDraftItem } from '../../services/demand'
import { ApiError } from '../../services/request'
import { getPayment, payOrder, waitPayment, type PaymentOverview } from '../../services/payment'
import { protectedPage } from '../../services/guard'
import { getCustomerOrder, getWorkOrderTimeline, acceptWorkOrder, rateWorkOrder, repeatCustomerOrder, type CustomerOrderDetail, type CustomerTimelineEvent, type CustomerWorkOrder } from '../../services/orders'
import { download } from '../../services/request'

type StageKey = 'ORDER_PLACED' | 'DISPATCHED' | 'PREPARING' | 'COMPLETED'
type FlowState = 'done' | 'current' | 'pending'

interface FlowStep {
  key: StageKey
  index: number
  label: string
  state: FlowState
  timeText: string
  description: string
  visible: boolean
  last: boolean
}

type CustomerWorkOrderView = CustomerWorkOrder & {
  flowSteps: FlowStep[]
  timelineSteps: FlowStep[]
  currentStageLabel: string
  appointmentText: string
}

type CustomerOrderView = Omit<CustomerOrderDetail, 'workOrders'> & {
  workOrders: CustomerWorkOrderView[]
  stageLabel: string
  totalAmountText: string
  createdAtText: string
  appointmentText: string
}

const stageMeta: Array<{ key: StageKey; label: string }> = [
  { key: 'ORDER_PLACED', label: '下单成功' },
  { key: 'DISPATCHED', label: '派单' },
  { key: 'PREPARING', label: '服务准备' },
  { key: 'COMPLETED', label: '服务完成' },
]

const stageIndexByStatus: Record<string, number> = {
  PENDING_PAYMENT: 0,
  PENDING_CONFIRMATION: 0,
  FULFILLING: 1,
  PENDING_DISPATCH: 0,
  PENDING_ACCEPT: 1,
  PENDING_ARRIVAL: 2,
  ARRIVED: 2,
  IN_SERVICE: 2,
  WAITING_COMPLETION_REVIEW: 3,
  WAITING_ACCEPTANCE: 3,
  WAITING_QA_AUDIT: 3,
  WAITING_DIRECTOR_AUDIT: 3,
  WAITING_CUSTOMER_SERVICE_CONFIRMATION: 3,
  SECOND_VISIT_PENDING: 3,
  REWORK_REQUIRED: 3,
  FINISHED: 3,
  FINISHED_WITH_REVIEW_EXCEPTION: 3,
  COMPLETED: 3,
  CANCELLED: 0,
}

const dispatchCodes = new Set(['ASSIGNED', 'REASSIGNED'])
const prepareCodes = new Set(['WORK_ORDER_ACCEPT', 'WORK_ORDER_ARRIVE', 'WORK_ORDER_START'])
const completedCodes = new Set([
  'COMPLETION_SUBMITTED',
  'COMPLETION_APPROVED',
  'CUSTOMER_ACCEPTED',
  'CUSTOMER_AUTO_ACCEPTED',
  'CUSTOMER_SERVICE_NO_SECOND_VISIT',
])

function pad(value: number) { return String(value).padStart(2, '0') }

function formatDateTime(value?: string) {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value.replace('T', ' ').slice(0, 16)
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`
}

function evidenceLabel(item: CustomerWorkOrder['evidence'][number]) {
  const stage = item.stage === 'BEFORE' ? '施工前' : item.stage === 'AFTER' ? '施工后' : item.stage
  return item.itemName && item.unitNo ? `${item.itemName} · 第${item.unitNo}个 · ${stage}` : stage
}

function firstEvent(events: CustomerTimelineEvent[], codes: Set<string>) {
  return events.find(event => codes.has(event.code))
}

function lastEvent(events: CustomerTimelineEvent[], codes: Set<string>) {
  return [...events].reverse().find(event => codes.has(event.code))
}

function currentStageIndex(work: CustomerWorkOrder) {
  // The current work-order status is authoritative. Historical events describe
  // what happened before a worker return or system recall and must not advance
  // the flow after the order has been sent back to dispatch.
  return Math.min(stageIndexByStatus[work.status] ?? 0, stageMeta.length - 1)
}

function buildFlow(work: CustomerWorkOrder, orderCreatedAt: string, events: CustomerTimelineEvent[]) {
  const current = currentStageIndex(work)
  const placed = firstEvent(events, new Set(['WORK_ORDER_CREATED']))
  const dispatched = lastEvent(events, dispatchCodes)
  const preparing = firstEvent(events, prepareCodes)
  const completed = firstEvent(events, completedCodes)
  const appointment = work.appointmentAt ? `${formatDateTime(work.appointmentAt)}${work.appointmentSlotLabel ? `（${work.appointmentSlotLabel}）` : ''}` : ''
  const descriptions: Record<StageKey, string> = {
    ORDER_PLACED: '您的服务订单已下单成功',
    DISPATCHED: dispatched?.note || (work.assigneeName ? `已安排服务师傅：${work.assigneeName}` : '已为您安排服务师傅'),
    PREPARING: work.assigneeName ? `${work.assigneeName}正在准备服务${appointment ? `，预约时间：${appointment}` : ''}` : `服务师傅正在准备${appointment ? `，预约时间：${appointment}` : ''}`,
    COMPLETED: work.completionSummary || '本次服务已完成，请及时验收并评价',
  }
  const times: Record<StageKey, string> = {
    ORDER_PLACED: formatDateTime(placed?.createdAt || orderCreatedAt),
    DISPATCHED: formatDateTime(dispatched?.createdAt),
    PREPARING: formatDateTime(preparing?.createdAt),
    COMPLETED: formatDateTime(completed?.createdAt),
  }
  const steps = stageMeta.map((stage, index): FlowStep => ({
    ...stage,
    index: index + 1,
    state: index < current ? 'done' : index === current ? 'current' : 'pending',
    timeText: times[stage.key],
    description: descriptions[stage.key],
    visible: index <= current,
    last: index === stageMeta.length - 1,
  }))
  return { steps, current, appointment }
}

const demandAttempts = new WeakMap<object, { signature: string; key: string }>()

protectedPage({
  data: { demand: null as OrderDemand | null, demandLoading: false, demandLoadError: '', showDemandEditor: false, editorItems: [] as DemandDraftItem[], demandDraft: [] as DemandDraftItem[], demandDirty: false, demandSaving: false, demandUploading: false, demandSaveError: '', demandConflict: false, previousDemand: [] as DemandDraftItem[], paying:false, payment:null as PaymentOverview|null, paymentLabel:'', loading: true, orderId: '', order: null as CustomerOrderView | null, acceptingWorkOrderId: '', reordering: false },

  onLoad(query: { id?: string; pay?:string }) {
    if (query.id) this.setData({ orderId: query.id }, () => this.load())
  },

  onShow() { if(this.data.orderId && !this.data.paying) void this.load() },

  async load() {
    try {
      const detail = await getCustomerOrder(this.data.orderId)
      await this.loadDemand()
      const payment=await getPayment(this.data.orderId)
      this.setData({payment,paymentLabel:({UNPAID:'待支付',EXEMPT:'历史订单免付',PAID:'已支付',PARTIALLY_REFUNDED:'部分退款',REFUNDED:'已退款',REFUNDING:'退款处理中'} as Record<string,string>)[payment.status]||payment.status})
      const workOrders = await Promise.all(detail.workOrders.map(async work => {
        const [timeline, evidence] = await Promise.all([
          getWorkOrderTimeline(work.id).catch(() => ({ items: [] as CustomerTimelineEvent[] })),
          Promise.all(work.evidence.map(async item => ({ ...item, url: await download(item.url), label: evidenceLabel(item) }))),
        ])
        const flow = buildFlow(work, detail.createdAt, timeline.items)
        const timelineSteps = flow.steps.filter(step => step.visible).map((step, index, visibleSteps) => ({ ...step, last: index === visibleSteps.length - 1 }))
        return {
          ...work,
          evidence,
          flowSteps: flow.steps,
          timelineSteps,
          currentStageLabel: stageMeta[flow.current].label,
          appointmentText: flow.appointment,
        }
      }))
      const stageIndex = workOrders.length ? Math.min(...workOrders.map(work => stageMeta.findIndex(stage => stage.label === work.currentStageLabel))) : 0
      const appointmentText = detail.appointmentAt ? `${formatDateTime(detail.appointmentAt)}${detail.appointmentSlotLabel ? `（${detail.appointmentSlotLabel}）` : ''}` : '待确认'
      const stageLabel = detail.status==='PENDING_CONFIRMATION' && payment.required && payment.paidAmount<detail.totalAmount ? '待支付' : detail.status === 'CANCELLED' ? (detail.statusText || '已取消') : detail.statusText === '已完成' ? '已完成' : stageMeta[stageIndex].label
      this.setData({ order: { ...detail, workOrders, stageLabel, totalAmountText: (detail.totalAmount / 100).toFixed(2), createdAtText: formatDateTime(detail.createdAt), appointmentText } })
    } catch (e) {
      wx.showToast({ title: e instanceof Error ? e.message : '订单加载失败', icon: 'none' })
    } finally {
      this.setData({ loading: false })
    }
  },

  async loadDemand() {
    this.setData({ demandLoading: true })
    try {
      const demand = await getOrderDemand(this.data.orderId)
      if (this.data.showDemandEditor || this.data.demandDirty || this.data.demandSaving) {
        // Keep the version that this draft was based on; otherwise a background refresh could hide a conflict.
        if (!demand.editable) this.setData({ demandConflict: true, demandSaveError: demand.readOnlyReason })
        return
      }
      this.setData({ demand, demandLoadError: '', demandDraft: demand.items, editorItems: demand.items })
    } catch { this.setData({ demandLoadError: '备注暂时无法加载，点击重试' }) }
    finally { this.setData({ demandLoading: false }) }
  },
  async openDemand() {
    if (this.data.demandSaving || this.data.demandLoading) return
    if (!this.data.demandDirty) await this.loadDemand()
    if (!this.data.demand || (!this.data.demandDirty && this.data.demandLoadError)) return
    if (!this.data.demand.editable) { wx.showToast({ title: this.data.demand.readOnlyReason, icon: 'none' }); return }
    this.setData({ editorItems: this.data.demandDraft, showDemandEditor: true })
  },
  demandChange(e: WechatMiniprogram.CustomEvent<{ items: DemandDraftItem[] }>) { this.setData({ demandDraft: e.detail.items, demandDirty: true }) },
  demandBusy(e: WechatMiniprogram.CustomEvent<{ busy: boolean }>) { this.setData({ demandUploading: e.detail.busy }) },
  closeDemand() { if (!this.data.demandSaving && !this.data.demandUploading) this.setData({ showDemandEditor: false }) },
  async refreshDemand() {
    const previousDemand = JSON.parse(JSON.stringify(this.data.demandDraft)) as DemandDraftItem[]
    try {
      const demand = await getOrderDemand(this.data.orderId)
      this.setData({ demand, previousDemand, editorItems: demand.items, demandDraft: demand.items, demandSaveError: '', demandConflict: false })
      demandAttempts.delete(this)
      if (!demand.editable) this.setData({ demandSaveError: demand.readOnlyReason, demandConflict: true })
    } catch { this.setData({ demandSaveError: '加载失败，上次输入已保留，请重试' }) }
  },
  async saveOrderDemand(e: WechatMiniprogram.CustomEvent<{ items: DemandDraftItem[] }>) {
    if (this.data.demandSaving || this.data.demandUploading || !this.data.demand?.editable || this.data.demandConflict) return
    this.setData({ demandSaving: true, demandSaveError: '', demandDraft: e.detail.items })
    const signature = JSON.stringify({ version: this.data.demand.demandVersion, items: e.detail.items.map(item => ({ id: item.itemId, text: item.demandDescription, media: item.demandMedia.map(media => media.id) })) })
    let attempt = demandAttempts.get(this)
    if (!attempt || attempt.signature !== signature) { attempt = { signature, key: `${Date.now()}-${Math.random().toString(16).slice(2)}` }; demandAttempts.set(this, attempt) }
    try {
      const demand = await updateOrderDemand(this.data.orderId, this.data.demand.demandVersion, e.detail.items, attempt.key)
      this.setData({ demand, demandDraft: demand.items, editorItems: demand.items, demandDirty: false, showDemandEditor: false, previousDemand: [] })
      demandAttempts.delete(this)
      wx.showToast({ title: '备注已保存', icon: 'success' }); await this.load()
    } catch (error) {
      const conflict = error instanceof ApiError && ['ORDER_DEMAND_VERSION_CONFLICT', 'ORDER_DEMAND_READ_ONLY'].includes(error.code)
      this.setData({ demandSaveError: error instanceof Error ? error.message : '保存失败，输入已保留，请重试', demandConflict: conflict })
    } finally { this.setData({ demandSaving: false }) }
  },
  async previewDemand(e: WechatMiniprogram.BaseEvent) {
    const item = this.data.demand?.items.find(item => item.itemId === String(e.currentTarget.dataset.item))
    const media = item?.demandMedia.find(media => media.id === String(e.currentTarget.dataset.media))
    if (!media) return
    try { const url = await download(`/api/v1/mini/media/${media.id}/content`); if (media.mediaType === 'VIDEO') wx.previewMedia({ sources: [{ url, type: 'video' }] }); else wx.previewImage({ urls: [url] }) }
    catch { wx.showToast({ title: '资料暂时无法预览，请重试', icon: 'none' }) }
  },

  async pay() {
    if(this.data.paying)return
    this.setData({paying:true})
    try { await payOrder(this.data.orderId);const result=await waitPayment(this.data.orderId);wx.showToast({title:result.paidAmount>0?'支付已确认':'支付结果确认中，请稍后刷新',icon:'none'}) }
    catch(e){wx.showToast({title:e instanceof Error?e.message:'支付未完成，请查询订单',icon:'none'})}
    finally{this.setData({paying:false});await this.load()}
  },

  async reorder() {
    const order = this.data.order
    if (!order || order.status !== 'CANCELLED' || !order.cancelReason || this.data.reordering) return
    const result = await new Promise<WechatMiniprogram.ShowModalSuccessCallbackResult>(resolve => wx.showModal({ title: '重新下单', content: '将原订单的服务和补充信息复制到购物车，当前购物车内容会保留，是否继续？', success: resolve }))
    if (!result.confirm) return
    this.setData({ reordering: true })
    try {
      const copied = await repeatCustomerOrder(order.id)
      wx.showToast({ title: `已复制${copied.itemsCopied}项服务`, icon: 'success' })
      setTimeout(() => wx.navigateTo({ url: '/pages/cart/index' }), 400)
    } catch (e) {
      wx.showToast({ title: e instanceof Error ? e.message : '重新下单失败', icon: 'none' })
    } finally {
      this.setData({ reordering: false })
    }
  },

  async acceptance(e: any) {
    const workId = e.currentTarget.dataset.workId as string
    const work = this.data.order?.workOrders.find(item => item.id === workId)
    if (!work || this.data.acceptingWorkOrderId === workId || (work.customerAcceptanceStatus && work.customerAcceptanceStatus !== 'PENDING')) return
    const decision = e.currentTarget.dataset.decision as 'ACCEPT' | 'REJECT'
    let reason = ''
    if (decision === 'REJECT') {
      const result = await new Promise<WechatMiniprogram.ShowModalSuccessCallbackResult>(resolve => wx.showModal({ title: '拒绝验收', editable: true, placeholderText: '请填写返工原因', success: resolve }))
      if (!result.confirm || !result.content?.trim()) return
      reason = result.content.trim()
    } else {
      const result = await new Promise<WechatMiniprogram.ShowModalSuccessCallbackResult>(resolve => wx.showModal({ title: '确认验收', content: '确认本次服务已完成吗？', success: resolve }))
      if (!result.confirm) return
    }
    this.setData({ acceptingWorkOrderId: work.id })
    try {
      await acceptWorkOrder(work.id, decision, work.version, reason)
      const workOrders = this.data.order?.workOrders.map(item => item.id === work.id ? { ...item, customerAcceptanceStatus: decision === 'ACCEPT' ? 'MANUAL_ACCEPTED' : 'REJECTED', version: item.version + 1 } : item) || []
      if (this.data.order) this.setData({ order: { ...this.data.order, workOrders } })
      wx.showToast({ title: decision === 'ACCEPT' ? '已确认完成' : '已提交返工', icon: 'success' })
      this.load()
    } catch (e) {
      wx.showToast({ title: e instanceof Error ? e.message : '操作失败', icon: 'none' })
    } finally {
      this.setData({ acceptingWorkOrderId: '' })
    }
  },

  async rating(e: any) {
    const workId = e.currentTarget.dataset.workId as string
    const work = this.data.order?.workOrders.find(item => item.id === workId)
    if (!work) return
    const result = await new Promise<WechatMiniprogram.ShowModalSuccessCallbackResult>(resolve => wx.showModal({ title: '服务评价', editable: true, placeholderText: '请输入1-5星及评价，例如：5 很满意', success: resolve }))
    if (!result.confirm || !result.content?.trim()) return
    const match = result.content.trim().match(/^([1-5])/)
    if (!match) {
      wx.showToast({ title: '请先输入1-5星', icon: 'none' })
      return
    }
    try {
      await rateWorkOrder(work.id, Number(match[1]), result.content.trim().slice(1).trim(), work.version)
      wx.showToast({ title: '评价已提交', icon: 'success' })
    } catch (e) {
      wx.showToast({ title: e instanceof Error ? e.message : '评价失败', icon: 'none' })
    }
  },
})

onSessionReset(() => {
  for (const page of getCurrentPages()) {
    if (page.route !== 'pages/orders/detail') continue
    demandAttempts.delete(page)
    page.setData({ order: null, demand: null, demandDraft: [], editorItems: [], previousDemand: [], showDemandEditor: false, demandDirty: false, demandSaving: false, demandUploading: false })
  }
})
