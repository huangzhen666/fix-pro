import { useState } from 'react'
import { Button, Card, Divider, Drawer, Input, Modal, Select, Space, Spin, Table, Tag, Typography, message } from 'antd'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { assignWorkOrder, internalReview, listWorkerCandidates, listWorkOrders } from '../api/fulfillment'
import { getOrder } from '../api/orders'
import { listSkills, listTrades } from '../api/workforce'
import { workOrderStatusLabel } from '../utils/enums'
import { OrderDetailContent } from '../components/OrderDetailDrawer'
import { FulfillmentDetailDrawer, type FulfillmentReviewOptions } from '../components/FulfillmentDetailDrawer'

const statuses = ['PENDING_DISPATCH', 'PENDING_ACCEPT', 'PENDING_ARRIVAL', 'ARRIVED', 'IN_SERVICE', 'WAITING_QA_AUDIT', 'WAITING_DIRECTOR_AUDIT', 'WAITING_CUSTOMER_SERVICE_CONFIRMATION', 'SECOND_VISIT_PENDING', 'FINISHED', 'FINISHED_WITH_REVIEW_EXCEPTION', 'CANCELLED']

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
  const client = useQueryClient()
  const orders = useQuery({ queryKey: ['work-orders', status, keyword, outcome, page], queryFn: () => listWorkOrders(status, keyword, outcome, page) })
  const trades = useQuery({ queryKey: ['dispatch-trades'], queryFn: () => listTrades('ACTIVE') })
  const skills = useQuery({ queryKey: ['dispatch-skills', tradeId], queryFn: () => listSkills(tradeId, 'ACTIVE'), enabled: Boolean(tradeId) })
  const workers = useQuery({ queryKey: ['worker-candidates', selected?.id, tradeId, skillId], queryFn: () => listWorkerCandidates(selected!.id, tradeId, skillId), enabled: Boolean(selected?.id) })
  const orderDetail = useQuery({ queryKey: ['order-for-dispatch', selected?.orderId], queryFn: () => getOrder(selected!.orderId), enabled: Boolean(selected?.orderId) })
  const assign = useMutation({ mutationFn: () => assignWorkOrder(selected.id, { workerId: workerId!, note: '', version: selected.version }), onSuccess: () => { message.success('派单成功'); setSelected(null); client.invalidateQueries({ queryKey: ['work-orders'] }) }, onError: (e: Error) => message.error(e.message) })
  const reviewMutation = useMutation({ mutationFn: () => internalReview(review.id, review.status === 'WAITING_DIRECTOR_AUDIT' ? 'DIRECTOR' : 'QA', { decision: review.decision, note: reviewNote, version: review.version }), onSuccess: () => { message.success('审核完成'); setReview(null); setReviewNote(''); client.invalidateQueries({ queryKey: ['work-orders'] }) }, onError: (e: Error) => message.error(e.message) })

  function openReview(row: any) {
    setReview({ ...row, decision: 'APPROVE' })
    setReviewNote('')
  }

  function submitReview() {
    if (review?.decision === 'REJECT' && !reviewNote.trim()) {
      message.warning('驳回原因必填')
      return
    }
    if (review?.decision === 'REJECT') {
      Modal.confirm({ title: '确认驳回完工？', content: '驳回后工单将退回服务中，师傅需要补充处理后重新提交。', okText: '确认驳回', cancelText: '取消', okButtonProps: { danger: true }, onOk: () => reviewMutation.mutate() })
      return
    }
    reviewMutation.mutate()
  }

  return <Card>
    <Space direction="vertical" size="large" style={{ width: '100%' }}>
      <Typography.Title level={3} style={{ margin: 0 }}>履约调度</Typography.Title>
      <Space><Select value={status} onChange={v => { setPage(1); setStatus(v) }} style={{ width: 220 }} options={[{ value: '', label: '全部状态' }, ...statuses.map(v => ({ value: v, label: workOrderStatusLabel(v) }))]} /><Select value={outcome} onChange={v => { setPage(1); setOutcome(v) }} style={{ width: 220 }} options={[{ value: '', label: '全部完工结果' }, { value: 'CUSTOMER_CONFIRMED_NO_SECOND_VISIT', label: '客户确认无需二次上门' }, { value: 'NORMAL', label: '正常完结' }]} /><Input.Search allowClear placeholder="工单号或订单号" onSearch={v => { setPage(1); setKeyword(v) }} style={{ width: 280 }} /></Space>
      <Table rowKey="id" loading={orders.isLoading} dataSource={orders.data?.items} pagination={{ current: page, total: orders.data?.total, pageSize: 20, showSizeChanger: false, onChange: p => setPage(p) }} columns={[
        { title: '工单号', dataIndex: 'workOrderNo' },
        { title: '订单号', dataIndex: 'orderNo' },
        { title: '状态', dataIndex: 'status', render: (v: string, row: any) => <Space><Tag color="blue">{workOrderStatusLabel(v)}</Tag>{row.completionOutcome === 'CUSTOMER_CONFIRMED_NO_SECOND_VISIT' ? <Tag color="green">客户确认无需二次上门</Tag> : null}</Space> },
        { title: '师傅', dataIndex: 'assigneeName', render: (v: string) => v || '待派单' },
        { title: '预约时间段', dataIndex: 'appointmentAt', render: (v: string, row: any) => v ? `${new Date(v).toLocaleDateString()} ${row.appointmentSlot || new Date(v).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}` : '-' },
        { title: '操作', render: (_: unknown, row: any) => <Space><Button type="link" onClick={() => setFulfillmentDrawerId(row.id)}>履约详情</Button>{row.status === 'PENDING_DISPATCH' ? <Button type="link" onClick={() => { setSelected(row); setWorkerId(undefined); setTradeId(''); setSkillId('') }}>派单</Button> : null}{row.status === 'WAITING_QA_AUDIT' || row.status === 'WAITING_DIRECTOR_AUDIT' ? <Button type="link" onClick={() => openReview(row)}>审核</Button> : null}</Space> },
      ]} />
    </Space>

    <Drawer title={orderDetail.data ? `派单 · 订单 ${orderDetail.data.order.orderNo}` : '派单'} open={!!selected} width={1100} onClose={() => setSelected(null)} footer={<Space style={{ width: '100%', justifyContent: 'flex-end' }}><Button onClick={() => setSelected(null)}>关闭</Button><Button type="primary" loading={assign.isPending} onClick={() => { if (!workerId) { message.warning('请选择师傅'); return } assign.mutate() }}>确认派单</Button></Space>}>
      {orderDetail.isLoading ? <Spin /> : orderDetail.data ? <Space direction="vertical" size="large" style={{ width: '100%' }}><OrderDetailContent data={orderDetail.data} /><Divider>师傅筛选与排班</Divider><Space><Select allowClear placeholder="工种" value={tradeId || undefined} onChange={v => { setTradeId(v || ''); setSkillId('') }} options={trades.data?.map(t => ({ value: t.id, label: t.name }))} style={{ width: 220 }} /><Select allowClear placeholder="技能" value={skillId || undefined} onChange={v => setSkillId(v || '')} options={skills.data?.map(s => ({ value: s.id, label: `${s.tradeName} / ${s.name}` }))} style={{ width: 280 }} /></Space><Typography.Text strong>可派师傅</Typography.Text><Space wrap>{workers.data?.map(w => <Card key={w.id} size="small" onClick={() => setWorkerId(w.id)} style={{ width: 320, borderColor: workerId === w.id ? '#1677ff' : undefined, cursor: 'pointer' }}><Typography.Text strong>{w.displayName}</Typography.Text><div>工种：{w.trades?.join('、') || '未配置'}</div><div>技能：{w.skills?.join('、') || '未配置'}</div><div>{w.appointmentAvailable ? '预约时段空闲' : '预约时段冲突'}　{w.allSkillsMatched ? '技能全匹配' : '部分匹配'}　未完成 {w.openWorkOrderCount} 单</div></Card>)}</Space></Space> : <Typography.Text type="danger">订单详情加载失败</Typography.Text>}
    </Drawer>
    <FulfillmentDetailDrawer open={Boolean(fulfillmentDrawerId || review)} workOrderId={review?.id || fulfillmentDrawerId} onClose={() => { if (review) { setReview(null); setReviewNote('') } else setFulfillmentDrawerId(undefined) }} review={review ? { level: review.status === 'WAITING_DIRECTOR_AUDIT' ? 'DIRECTOR' : 'QA', decision: review.decision, note: reviewNote, loading: reviewMutation.isPending, onDecisionChange: decision => setReview((current: any) => ({ ...current, decision })), onNoteChange: setReviewNote, onSubmit: submitReview } satisfies FulfillmentReviewOptions : undefined} />
  </Card>
}
