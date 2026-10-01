import { useState } from 'react'
import { Button, Card, Divider, Drawer, Input, Modal, Select, Space, Spin, Table, Tag, Typography, message } from 'antd'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { assignWorkOrder, internalReview, listWorkerCandidates, listWorkOrders, recallWorkOrder, rescheduleWorkOrder } from '../api/fulfillment'
import { getOrder } from '../api/orders'
import { listSkills, listTrades } from '../api/workforce'
import { adminWorkOrderStatusLabel, workOrderStatusLabel } from '../utils/enums'
import { OrderDetailContent } from '../components/OrderDetailDrawer'
import { FulfillmentDetailDrawer, type FulfillmentReviewOptions } from '../components/FulfillmentDetailDrawer'

const statuses = ['PENDING_DISPATCH', 'PENDING_ACCEPT', 'PENDING_ARRIVAL', 'ARRIVED', 'IN_SERVICE', 'WAITING_QA_AUDIT', 'WAITING_DIRECTOR_AUDIT', 'WAITING_CUSTOMER_SERVICE_CONFIRMATION', 'SECOND_VISIT_PENDING', 'FINISHED', 'FINISHED_WITH_REVIEW_EXCEPTION', 'CANCELLED']
const appointmentSlots = ['08:00', '10:00', '12:00', '14:00', '16:00', '18:00', '20:00']

function appointmentDeadline(appointmentAt?: string, appointmentSlot?: string) {
  if (!appointmentAt || !appointmentSlot) return undefined
  const hour = Number(appointmentSlot.slice(0, 2))
  const date = new Date(appointmentAt)
  if (!Number.isInteger(hour) || Number.isNaN(date.getTime())) return undefined
  return new Date(date.getFullYear(), date.getMonth(), date.getDate(), hour + 2, 0, 0, 0).getTime()
}

function appointmentExpired(appointmentAt?: string, appointmentSlot?: string) {
  const deadline = appointmentDeadline(appointmentAt, appointmentSlot)
  return deadline !== undefined && deadline <= Date.now()
}

function dateInputValue(value?: string) {
  const date = value ? new Date(value) : new Date()
  const pad = (number: number) => String(number).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`
}

function appointmentText(appointmentAt?: string, appointmentSlot?: string) {
  if (!appointmentAt) return '-'
  const date = new Date(appointmentAt)
  if (Number.isNaN(date.getTime())) return appointmentAt
  const pad = (number: number) => String(number).padStart(2, '0')
  const day = `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`
  if (!appointmentSlot) return day
  const hour = Number(appointmentSlot.slice(0, 2))
  const slot = Number.isInteger(hour) ? `${appointmentSlot}–${String(hour + 2).padStart(2, '0')}:00` : appointmentSlot
  return `${day} · ${slot}`
}

function acceptanceTiming(status: string, appointmentAt?: string, appointmentSlot?: string) {
  if (status !== 'PENDING_ACCEPT' || !appointmentAt || !appointmentSlot) return undefined
  const deadline = appointmentDeadline(appointmentAt, appointmentSlot)
  if (deadline === undefined) return undefined
  const remaining = deadline - Date.now()
  if (remaining <= 0) return { expired: true, label: '接单已超时' }
  if (remaining <= 30 * 60 * 1000) return { expired: false, label: `接单即将超时（剩余 ${Math.ceil(remaining / 60000)} 分钟）` }
  return undefined
}

export default function WorkOrderPage() {
  const [status, setStatus] = useState('')
  const [keyword, setKeyword] = useState('')
  const [outcome, setOutcome] = useState('')
  const [page, setPage] = useState(1)
  const [selected, setSelected] = useState<any>(null)
  const [workerId, setWorkerId] = useState<string>()
  const [tradeId, setTradeId] = useState('')
  const [skillId, setSkillId] = useState('')
  const [review, setReview] = useState<any>(null)
  const [fulfillmentDrawerId, setFulfillmentDrawerId] = useState<string>()
  const [reviewNote, setReviewNote] = useState('')
  const [recalling, setRecalling] = useState<any>(null)
  const [recallReason, setRecallReason] = useState('')
  const [rescheduling, setRescheduling] = useState<any>(null)
  const [rescheduleDate, setRescheduleDate] = useState('')
  const [rescheduleSlot, setRescheduleSlot] = useState('08:00')
  const client = useQueryClient()
  const orders = useQuery({ queryKey: ['work-orders', status, keyword, outcome, page], queryFn: () => listWorkOrders(status, keyword, outcome, page), refetchInterval: 60_000 })
  const trades = useQuery({ queryKey: ['dispatch-trades'], queryFn: () => listTrades('ACTIVE') })
  const skills = useQuery({ queryKey: ['dispatch-skills', tradeId], queryFn: () => listSkills(tradeId, 'ACTIVE'), enabled: Boolean(tradeId) })
  const workers = useQuery({ queryKey: ['worker-candidates', selected?.id, tradeId, skillId], queryFn: () => listWorkerCandidates(selected!.id, tradeId, skillId), enabled: Boolean(selected?.id) })
  const orderDetail = useQuery({ queryKey: ['order-for-dispatch', selected?.orderId], queryFn: () => getOrder(selected!.orderId), enabled: Boolean(selected?.orderId) })
  const assign = useMutation({ mutationFn: () => assignWorkOrder(selected.id, { workerId: workerId!, note: '', version: selected.version }), onSuccess: () => { message.success('派单成功'); setSelected(null); client.invalidateQueries({ queryKey: ['work-orders'] }) }, onError: (e: Error) => message.error(e.message) })
  const reviewMutation = useMutation({ mutationFn: (decision: 'APPROVE' | 'REJECT') => internalReview(review.id, review.status === 'WAITING_DIRECTOR_AUDIT' ? 'DIRECTOR' : 'QA', { decision, note: reviewNote, version: review.version }), onSuccess: () => { message.success('审核完成'); setReview(null); setReviewNote(''); client.invalidateQueries({ queryKey: ['work-orders'] }) }, onError: (e: Error) => message.error(e.message) })
  const recall = useMutation({ mutationFn: () => recallWorkOrder(recalling.id, { reason: recallReason.trim(), version: recalling.version }), onSuccess: () => { message.success('已收回派单，等待重新派单'); setRecalling(null); setRecallReason(''); client.invalidateQueries({ queryKey: ['work-orders'] }) }, onError: (e: Error) => message.error(e.message) })
  const reschedule = useMutation({ mutationFn: () => rescheduleWorkOrder(rescheduling.id, { appointmentAt: new Date(`${rescheduleDate}T00:00:00`).toISOString(), appointmentSlot: rescheduleSlot, version: rescheduling.version }), onSuccess: () => { message.success('预约已更新，现在可以派单'); setRescheduling(null); client.invalidateQueries({ queryKey: ['work-orders'] }) }, onError: (e: Error) => message.error(e.message) })

  function openReview(row: any) {
    setReview({ ...row })
    setReviewNote('')
  }

  function submitReview(decision: 'APPROVE' | 'REJECT') {
    if (decision === 'REJECT' && !reviewNote.trim()) {
      message.warning('驳回原因必填')
      return
    }
    if (decision === 'REJECT') {
      Modal.confirm({ title: '确认打回完工？', content: '打回后工单将退回服务中，师傅需要补充处理后重新提交。', okText: '确认打回', cancelText: '取消', okButtonProps: { danger: true }, onOk: () => reviewMutation.mutate(decision) })
      return
    }
    reviewMutation.mutate(decision)
  }

  return <Card className="work-order-page" styles={{ body: { padding: '16px 18px' } }}>
    <Space className="work-order-page-content" direction="vertical" size={14} style={{ width: '100%' }}>
      <Typography.Title level={3} style={{ margin: 0, fontSize: 26 }}>履约调度</Typography.Title>
      <Space className="work-order-filters" size={8} wrap>
        <Select value={status} onChange={v => { setPage(1); setStatus(v) }} style={{ width: 190 }} options={[{ value: '', label: '全部状态' }, ...statuses.map(v => ({ value: v, label: workOrderStatusLabel(v) }))]} />
        <Select value={outcome} onChange={v => { setPage(1); setOutcome(v) }} style={{ width: 210 }} options={[{ value: '', label: '全部完工结果' }, { value: 'CUSTOMER_CONFIRMED_NO_SECOND_VISIT', label: '客户确认无需二次上门' }, { value: 'NORMAL', label: '正常完结' }]} />
        <Input.Search allowClear placeholder="搜索工单号或订单号" onSearch={v => { setPage(1); setKeyword(v) }} style={{ width: 280 }} />
      </Space>
      <Table className="work-order-table" size="small" tableLayout="fixed" rowKey="id" loading={orders.isLoading} dataSource={orders.data?.items} pagination={{ current: page, total: orders.data?.total, pageSize: 20, showSizeChanger: false, onChange: p => setPage(p) }} columns={[
        { title: '工单号', dataIndex: 'workOrderNo', width: 220, ellipsis: true },
        { title: '订单号', dataIndex: 'orderNo', width: 220, ellipsis: true },
        { title: '履约信息', dataIndex: 'status', render: (v: string, row: any) => { const timing = acceptanceTiming(v, row.appointmentAt, row.appointmentSlot); const expiredDispatch = v === 'PENDING_DISPATCH' && appointmentExpired(row.appointmentAt, row.appointmentSlot); return <div className="work-order-fulfillment"><Space className="work-order-status" size={[4, 4]} wrap><Tag color={expiredDispatch ? 'volcano' : 'blue'}>{expiredDispatch ? '预约已过期' : adminWorkOrderStatusLabel(v, row.customerAcceptanceStatus)}</Tag>{timing ? <Tag color={timing.expired ? 'volcano' : 'gold'}>{timing.label}</Tag> : null}{row.completionOutcome === 'CUSTOMER_CONFIRMED_NO_SECOND_VISIT' ? <Tag color="green">客户确认无需二次上门</Tag> : null}</Space><div className="work-order-meta"><span>师傅：{row.assigneeName || '待派单'}</span><span>预约：{appointmentText(row.appointmentAt, row.appointmentSlot)}</span></div></div> } },
        { title: '操作', width: 150, render: (_: unknown, row: any) => { const expiredDispatch = row.status === 'PENDING_DISPATCH' && appointmentExpired(row.appointmentAt, row.appointmentSlot); return <Space className="work-order-actions" size={0}><Button type="link" onClick={() => setFulfillmentDrawerId(row.id)}>详情</Button>{row.status === 'PENDING_DISPATCH' && !expiredDispatch ? <Button type="link" onClick={() => { setSelected(row); setWorkerId(undefined); setTradeId(''); setSkillId('') }}>派单</Button> : null}{expiredDispatch ? <Button type="link" danger onClick={() => { setRescheduling({ ...row }); setRescheduleDate(dateInputValue(row.appointmentAt)); setRescheduleSlot(row.appointmentSlot || '08:00') }}>重新预约</Button> : null}{row.status === 'PENDING_ACCEPT' ? <Button type="link" danger onClick={() => { setRecalling({ ...row }); setRecallReason('') }}>收回</Button> : null}{row.status === 'WAITING_QA_AUDIT' || row.status === 'WAITING_DIRECTOR_AUDIT' ? <Button type="link" onClick={() => openReview(row)}>审核</Button> : null}</Space> } },
      ]} />
    </Space>

    <Drawer title={orderDetail.data ? `派单 · 订单 ${orderDetail.data.order.orderNo}` : '派单'} open={!!selected} width={1100} onClose={() => setSelected(null)} footer={<Space style={{ width: '100%', justifyContent: 'flex-end' }}><Button onClick={() => setSelected(null)}>关闭</Button><Button type="primary" loading={assign.isPending} onClick={() => { if (!workerId) { message.warning('请选择师傅'); return } assign.mutate() }}>确认派单</Button></Space>}>
      {orderDetail.isLoading ? <Spin /> : orderDetail.data ? <Space direction="vertical" size="large" style={{ width: '100%' }}><OrderDetailContent data={orderDetail.data} /><Divider>师傅筛选与排班</Divider><Space><Select allowClear placeholder="工种" value={tradeId || undefined} onChange={v => { setTradeId(v || ''); setSkillId('') }} options={trades.data?.map(t => ({ value: t.id, label: t.name }))} style={{ width: 220 }} /><Select allowClear placeholder="技能" value={skillId || undefined} onChange={v => setSkillId(v || '')} options={skills.data?.map(s => ({ value: s.id, label: `${s.tradeName} / ${s.name}` }))} style={{ width: 280 }} /></Space><Typography.Text strong>可派师傅</Typography.Text><Space wrap>{workers.data?.map(w => <Card key={w.id} size="small" onClick={() => setWorkerId(w.id)} style={{ width: 320, borderColor: workerId === w.id ? '#1677ff' : undefined, cursor: 'pointer' }}><Typography.Text strong>{w.displayName}</Typography.Text><div>工种：{w.trades?.join('、') || '未配置'}</div><div>技能：{w.skills?.join('、') || '未配置'}</div><div>{w.appointmentAvailable ? '预约时段空闲' : '预约时段冲突'}　{w.allSkillsMatched ? '技能全匹配' : '部分匹配'}　未完成 {w.openWorkOrderCount} 单</div></Card>)}</Space></Space> : <Typography.Text type="danger">订单详情加载失败</Typography.Text>}
    </Drawer>
    <Modal title="收回派单" open={Boolean(recalling)} okText="确认收回" cancelText="取消" okButtonProps={{ danger: true }} confirmLoading={recall.isPending} onCancel={() => { setRecalling(null); setRecallReason('') }} onOk={() => { if (!recallReason.trim()) { message.warning('请填写收回原因'); return } recall.mutate() }}>
      <Space direction="vertical" size="middle" style={{ width: '100%' }}>
        <Typography.Text>收回后工单将回到待派单，客户预约时间段保持不变。</Typography.Text>
        <Input.TextArea rows={4} placeholder="请输入收回原因（必填）" value={recallReason} onChange={event => setRecallReason(event.target.value)} />
      </Space>
    </Modal>
    <Modal title="重新预约" open={Boolean(rescheduling)} okText="保存预约" cancelText="取消" confirmLoading={reschedule.isPending} onCancel={() => setRescheduling(null)} onOk={() => { if (!rescheduleDate) { message.warning('请选择预约日期'); return } reschedule.mutate() }}>
      <Space direction="vertical" size="middle" style={{ width: '100%' }}>
        <Typography.Text>原预约时段已结束，更新预约后才可重新派单。</Typography.Text>
        <Input type="date" value={rescheduleDate} onChange={event => setRescheduleDate(event.target.value)} />
        <Select value={rescheduleSlot} onChange={setRescheduleSlot} options={appointmentSlots.map(slot => ({ value: slot, label: `${slot}-${String(Number(slot.slice(0, 2)) + 2).padStart(2, '0')}:00` }))} />
      </Space>
    </Modal>
    <FulfillmentDetailDrawer open={Boolean(fulfillmentDrawerId || review)} workOrderId={review?.id || fulfillmentDrawerId} onClose={() => { if (review) { setReview(null); setReviewNote('') } else setFulfillmentDrawerId(undefined) }} review={review ? { level: review.status === 'WAITING_DIRECTOR_AUDIT' ? 'DIRECTOR' : 'QA', note: reviewNote, loading: reviewMutation.isPending, onNoteChange: setReviewNote, onSubmit: submitReview } satisfies FulfillmentReviewOptions : undefined} />
  </Card>
}
