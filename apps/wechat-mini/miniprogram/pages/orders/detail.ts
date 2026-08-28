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

function firstEvent(events: CustomerTimelineEvent[], codes: Set<string>) {
  return events.find(event => codes.has(event.code))
}

function lastEvent(events: CustomerTimelineEvent[], codes: Set<string>) {
  return [...events].reverse().find(event => codes.has(event.code))
}

function currentStageIndex(work: CustomerWorkOrder, events: CustomerTimelineEvent[]) {
  let index = stageIndexByStatus[work.status] ?? 0
  if (events.some(event => dispatchCodes.has(event.code))) index = Math.max(index, 1)
  if (events.some(event => prepareCodes.has(event.code))) index = Math.max(index, 2)
  if (events.some(event => completedCodes.has(event.code))) index = 3
  return Math.min(index, stageMeta.length - 1)
}

function buildFlow(work: CustomerWorkOrder, orderCreatedAt: string, events: CustomerTimelineEvent[]) {
  const current = currentStageIndex(work, events)
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

Page({
  data: { loading: true, orderId: '', order: null as CustomerOrderView | null, acceptingWorkOrderId: '', reordering: false },

  onLoad(query: { id?: string }) {
    if (query.id) this.setData({ orderId: query.id }, () => this.load())
  },

  async load() {
    try {
      const detail = await getCustomerOrder(this.data.orderId)
      const workOrders = await Promise.all(detail.workOrders.map(async work => {
        const [timeline, evidence] = await Promise.all([
          getWorkOrderTimeline(work.id).catch(() => ({ items: [] as CustomerTimelineEvent[] })),
          Promise.all(work.evidence.map(async item => ({ ...item, url: await download(item.url) }))),
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
      const stageLabel = detail.status === 'CANCELLED' ? (detail.statusText || '已取消') : detail.statusText === '已完成' ? '已完成' : stageMeta[stageIndex].label
      this.setData({ order: { ...detail, workOrders, stageLabel, totalAmountText: (detail.totalAmount / 100).toFixed(2), createdAtText: formatDateTime(detail.createdAt), appointmentText } })
    } catch (e) {
      wx.showToast({ title: e instanceof Error ? e.message : '订单加载失败', icon: 'none' })
    } finally {
      this.setData({ loading: false })
    }
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
