import { useEffect, useState } from 'react'
import { Button, Drawer, Descriptions, Divider, Empty, Image, Input, Radio, Space, Spin, Tag, Timeline, Typography } from 'antd'
import { useQuery } from '@tanstack/react-query'
import { getOrder } from '../api/orders'
import { getWorkOrderTimeline, getWorkOrder, type AdminEvidence, type AdminInternalReview } from '../api/fulfillment'
import { apiBlob } from '../api/http'
import { workOrderStatusLabel } from '../utils/enums'
import { OrderDetailContent } from './OrderDetailDrawer'

const timelineLabels: Record<string, string> = {
  WORK_ORDER_CREATED: '工单创建',
  ASSIGNED: '已派单',
  REASSIGNED: '已改派',
  WORK_ORDER_ACCEPT: '师傅已接单',
  WORK_ORDER_REJECT: '师傅拒单',
  WORK_ORDER_ARRIVE: '师傅已到达',
  WORK_ORDER_START: '已开始服务',
  WORKER_RESCHEDULE_REQUESTED: '师傅发起改期',
  RESCHEDULED: '已改期',
  WORKER_RETURNED: '师傅退回调度',
  ACCEPTED: '师傅已接单',
  ARRIVED: '师傅已到达',
  STARTED: '已开始服务',
  COMPLETION_SUBMITTED: '师傅提交完工',
  COMPLETION_APPROVED: '完工审核通过',
  COMPLETION_REJECTED: '完工审核驳回',
  CUSTOMER_ACCEPTED: '客户人工验收通过',
  CUSTOMER_REJECTED: '客户验收驳回',
  CUSTOMER_ACCEPTANCE_ACCEPT: '客户人工验收通过',
  CUSTOMER_ACCEPTANCE_REJECT: '客户验收驳回',
  CUSTOMER_AUTO_ACCEPTED: '系统自动验收通过',
  CUSTOMER_RATING: '客户评价',
  INTERNAL_QA_APPROVE: '质检初审通过',
  INTERNAL_QA_REJECT: '质检初审驳回',
  INTERNAL_DIRECTOR_APPROVE: '总监审核通过',
  INTERNAL_DIRECTOR_REJECT: '总监审核驳回',
  QA_APPROVED: '质检初审通过',
  QA_REJECTED: '质检初审驳回',
  DIRECTOR_APPROVED: '总监审核通过',
  DIRECTOR_REJECTED: '总监审核驳回',
  CUSTOMER_SERVICE_SECOND_VISIT_REQUIRED: '客服确认二次上门',
  CUSTOMER_SERVICE_NO_SECOND_VISIT: '客服确认无需二次上门',
}

function timelineLabel(code: string) { return timelineLabels[code] || code }
function formatAt(value?: string) { return value ? new Date(value).toLocaleString() : '-' }
function formatDateTime(value: string) {
  const date = new Date(value)
  const pad = (number: number) => String(number).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
}
function appointmentText(date?: string, slot?: string) {
  if (!date) return '-'
  const hour = slot ? Number(slot.slice(0, 2)) : NaN
  const range = Number.isFinite(hour) ? `${slot}-${String(hour + 2).padStart(2, '0')}:00` : (slot || '')
  return `${new Date(date).toLocaleDateString()} ${range}`.trim()
}
const acceptanceLabels: Record<string, string> = { PENDING: '待客户验收', MANUAL_ACCEPTED: '客户人工通过', AUTO_ACCEPTED: '系统自动通过', REJECTED: '客户驳回' }
const reviewLabels: Record<string, string> = { PENDING_QA: '待质检初审', PENDING_DIRECTOR: '待总监审核', APPROVED: '审核通过', REJECTED: '审核不通过' }
const closureLabels: Record<string, string> = { OPEN: '履约中', WAITING_CUSTOMER_SERVICE_CONFIRMATION: '待客服确认', SECOND_VISIT_PENDING: '待二次上门', FINISHED_WITH_REVIEW_EXCEPTION: '审核异常已完结', FINISHED: '已完结' }
const operatorLabels: Record<string, string> = { ADMIN: '后台人员', WORKER: '维修师傅', CUSTOMER: '客户', SYSTEM: '系统' }
const reviewLevelLabels: Record<string, string> = { QA: '质检初审', DIRECTOR: '总监审核' }
const reviewDecisionLabels: Record<string, string> = { APPROVE: '通过', REJECT: '驳回' }

export interface FulfillmentReviewOptions {
  level: 'QA' | 'DIRECTOR'
  decision: 'APPROVE' | 'REJECT'
  note: string
  loading?: boolean
  onDecisionChange: (decision: 'APPROVE' | 'REJECT') => void
  onNoteChange: (note: string) => void
  onSubmit: () => void
}

function Evidence({ item }: { item: AdminEvidence }) { const [src, setSrc] = useState(''); useEffect(() => { let u=''; apiBlob(item.url).then(b => { u=URL.createObjectURL(b); setSrc(u) }); return () => { if (u) URL.revokeObjectURL(u) } }, [item.url]); return src ? (item.mediaType === 'VIDEO' ? <video controls src={src} style={{ width: 220, maxHeight: 160 }} /> : <Image width={180} src={src} />) : <Typography.Text type="secondary">加载凭证中</Typography.Text> }
function reviewTag(review: AdminInternalReview) { return <Tag color={review.decision === 'APPROVE' ? 'success' : 'error'}>{reviewDecisionLabels[review.decision] || review.decision}</Tag> }
function reviewerText(review: AdminInternalReview) { return review.reviewerName || (review.reviewerId === '0' ? '后台管理员' : review.reviewerId) }

export function FulfillmentDetailDrawer({ open, workOrderId, onClose, review }: { open: boolean; workOrderId?: string; onClose: () => void; review?: FulfillmentReviewOptions }) {
  const work = useQuery({ queryKey: ['fulfillment-drawer-work-order', workOrderId], queryFn: () => getWorkOrder(workOrderId!), enabled: open && Boolean(workOrderId) })
  const order = useQuery({ queryKey: ['fulfillment-drawer-order', work.data?.orderId], queryFn: () => getOrder(work.data!.orderId), enabled: open && Boolean(work.data?.orderId) })
  const timeline = useQuery({ queryKey: ['fulfillment-drawer-timeline', workOrderId], queryFn: () => getWorkOrderTimeline(workOrderId!), enabled: open && Boolean(workOrderId) })
  const qaReview = work.data?.internalReviews ? [...work.data.internalReviews].reverse().find(item => item.level === 'QA') : undefined
  const footer = review ? <Space style={{ width: '100%', justifyContent: 'flex-end' }}><Button onClick={onClose}>关闭</Button><Button type="primary" danger={review.decision === 'REJECT'} loading={review.loading} onClick={review.onSubmit}>{review.decision === 'REJECT' ? '确认驳回' : '确认通过'}</Button></Space> : undefined
  return <Drawer title={work.data ? `${review ? reviewLevelLabels[review.level] : '履约详情'} · ${work.data.workOrderNo}` : review ? reviewLevelLabels[review.level] : '履约详情'} open={open} onClose={onClose} width={1200} footer={footer}>
    {work.isLoading ? <Spin /> : !work.data ? <Empty description="履约详情加载失败" /> : <Space direction="vertical" size="large" style={{ width: '100%' }}>
      <Divider>履约全流程</Divider>{timeline.isLoading ? <Spin /> : timeline.isError ? <Typography.Text type="danger">流程记录加载失败，请稍后重试</Typography.Text> : timeline.data?.items.length ? <Timeline items={timeline.data.items.map(item => ({ children: <Space direction="vertical" size={2}><Typography.Text strong>{timelineLabel(item.code)}</Typography.Text>{timelineLabels[item.code] ? <Typography.Text type="secondary">操作编码：{item.code}</Typography.Text> : null}<Typography.Text>执行时间：{formatDateTime(item.createdAt)}</Typography.Text><Typography.Text>执行主体：{operatorLabels[item.operatorType] || item.operatorType}</Typography.Text>{item.note ? <Typography.Text>操作说明：{item.note}</Typography.Text> : null}</Space> }))} /> : <Empty description="暂无流程记录" />}
      <Typography.Title level={5}>订单与客户信息</Typography.Title>{order.isLoading ? <Spin /> : order.isError ? <Typography.Text type="danger">订单详情加载失败，请检查后端服务</Typography.Text> : order.data ? <OrderDetailContent data={order.data} /> : <Empty description="暂无订单详情" />}
      {review ? <><Divider>审核关键信息</Divider><Descriptions bordered size="small" column={2} items={[{ key: 'level', label: '本次审核层级', children: reviewLevelLabels[review.level] }, { key: 'status', label: '当前履约状态', children: <Tag color="blue">{workOrderStatusLabel(work.data.status)}</Tag> }, { key: 'acceptance', label: '客户验收结果', children: acceptanceLabels[work.data.customerAcceptanceStatus] || work.data.customerAcceptanceStatus || '-' }, { key: 'outcome', label: '完工结果', children: work.data.completionOutcome || '-' }, { key: 'qa', label: '质检初审结果', span: 2, children: review.level === 'DIRECTOR' ? (qaReview ? <Space direction="vertical" size={2}><Space>{reviewTag(qaReview)}<Typography.Text>{reviewerText(qaReview)}</Typography.Text><Typography.Text type="secondary">{formatDateTime(qaReview.createdAt)}</Typography.Text></Space><Typography.Text>{qaReview.note || '初审未填写备注'}</Typography.Text></Space> : <Typography.Text type="secondary">未找到质检初审记录</Typography.Text>) : <Typography.Text type="secondary">本次为质检初审，审核结果将在提交后记录</Typography.Text> }]} /><Typography.Title level={5} style={{ marginTop: 0 }}>本次审核决定</Typography.Title><Radio.Group value={review.decision} onChange={event => review.onDecisionChange(event.target.value)} options={[{ value: 'APPROVE', label: '通过' }, { value: 'REJECT', label: '驳回' }]} /><Input.TextArea rows={4} placeholder={review.decision === 'REJECT' ? '请输入驳回原因（必填）' : '审核备注（可选）'} value={review.note} onChange={event => review.onNoteChange(event.target.value)} /></> : null}
      <Divider>当前履约状态</Divider><Descriptions bordered size="small" column={2} items={[{ key: 'status', label: '工单状态', children: <Tag color="blue">{workOrderStatusLabel(work.data.status)}</Tag> }, { key: 'worker', label: '当前师傅', children: work.data.assigneeName || '待派单' }, { key: 'appointment', label: '客户预约时间段', children: appointmentText(work.data.appointmentAt, work.data.appointmentSlot) }, { key: 'visit', label: '上门状态', children: work.data.visitStatus || '-' }, { key: 'acceptance', label: '客户验收', children: acceptanceLabels[work.data.customerAcceptanceStatus] || work.data.customerAcceptanceStatus || '-' }, { key: 'acceptanceAt', label: '验收时间', children: formatAt(work.data.customerAcceptanceAt) }, { key: 'review', label: '内部审核', children: reviewLabels[work.data.internalReviewStatus] || work.data.internalReviewStatus || '-' }, { key: 'closure', label: '结案状态', children: closureLabels[work.data.closureStatus] || work.data.closureStatus || '-' }, { key: 'outcome', label: '完工结果', children: work.data.completionOutcome || '-' }, { key: 'summary', label: '完工说明', span: 2, children: work.data.completionSummary || '暂无' }]} />
      <Divider>上门与完工节点</Divider><Descriptions bordered size="small" column={2} items={[{ key: 'acceptedAt', label: '师傅接单', children: formatAt(work.data.acceptedAt) }, { key: 'arrivedAt', label: '师傅到达', children: formatAt(work.data.arrivedAt) }, { key: 'startedAt', label: '开始服务', children: formatAt(work.data.startedAt) }, { key: 'completionSubmittedAt', label: '提交完工', children: formatAt(work.data.completionSubmittedAt || work.data.completionSubmissionAt) }, { key: 'reviewedAt', label: '审核完成', children: formatAt(work.data.reviewedAt) }, { key: 'finishedAt', label: '工单完结', children: formatAt(work.data.finishedAt || work.data.closedAt) }]} />
      <Divider>施工凭证</Divider><Space wrap>{work.data.evidence.length ? work.data.evidence.map(item => <div key={item.id}><Typography.Text>{item.stage} · {item.customerVisible ? '客户可见' : '内部可见'}</Typography.Text><br /><Evidence item={item} /></div>) : <Typography.Text type="secondary">暂无凭证</Typography.Text>}</Space>
    </Space>}
  </Drawer>
}
