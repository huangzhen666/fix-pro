import { useEffect, useState } from 'react'
import { Button, Card, Descriptions, Divider, Drawer, Empty, Input, Modal, Space, Spin, Tag, Typography, message } from 'antd'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { confirmAndCreateWorkOrders, getOrder, rejectOrder } from '../api/orders'
import { AuthMedia } from './AuthMedia'
import { orderStatusLabel } from '../utils/enums'

function formatAppointment(date?: string, slot?: string) {
  if (!date) return '未设置'
  const hour = slot ? Number(slot.slice(0, 2)) : NaN
  const range = Number.isFinite(hour) ? `${slot}-${String(hour + 2).padStart(2, '0')}:00` : (slot || '')
  return `${new Date(date).toLocaleDateString()} ${range}`.trim()
}

export function OrderDetailContent({ data, statusOverride }: { data: Awaited<ReturnType<typeof getOrder>>; statusOverride?: string }) {
  const orderLabel = data.order.status === 'CANCELLED' && data.order.cancelReason ? '商家已打回' : orderStatusLabel(data.order.status)
  const orderItems = [{ key: 'status', label: '状态', children: <Tag color="gold">{statusOverride || orderLabel}</Tag> }, ...(data.order.cancelReason ? [{ key: 'cancelReason', label: '打回原因', span: 2, children: data.order.cancelReason }] : []), { key: 'amount', label: '总金额', children: `¥${(data.order.totalAmount / 100).toFixed(2)}` }, { key: 'name', label: '联系人', children: data.order.contactName }, { key: 'mobile', label: '手机号', children: data.order.contactMobile }, { key: 'address', label: '服务地址', span: 2, children: data.order.serviceAddress }, { key: 'appointment', label: '客户预约', span: 2, children: formatAppointment(data.order.appointmentAt, data.order.appointmentSlot) }, { key: 'created', label: '下单时间', children: new Date(data.order.createdAt).toLocaleString() }]
  return <Space direction="vertical" size="middle" style={{ width: '100%' }}><Descriptions column={2} bordered size="small" items={orderItems} />{data.items.map(item => <Card key={item.id} size="small" title={`${item.skuName} · V${item.skuVersion}`}><Descriptions column={1} size="small" items={[{ key: 'code', label: 'SKU 编码', children: item.skuCode }, { key: 'scope', label: '服务范围', children: item.serviceScope }, { key: 'exclusions', label: '除外项', children: item.exclusions }, { key: 'warranty', label: '售后/质保', children: item.warrantyDescription }, { key: 'fault', label: '故障描述', children: item.faultDescription || '未填写' }, { key: 'price', label: '成交金额', children: `¥${(item.unitPrice / 100).toFixed(2)} × ${item.quantity}${item.unit} = ¥${(item.subtotal / 100).toFixed(2)}` }]} /><Divider>客户故障资料（{item.faultMedia.length}）</Divider><Space wrap>{item.faultMedia.map(m => <AuthMedia key={m.id} url={m.url} type={m.mediaType} name={m.name} />)}</Space></Card>)}</Space>
}

export function OrderDetailDrawer({ open, orderId, onClose }: { open: boolean; orderId?: string; onClose: () => void }) {
  const client = useQueryClient()
  const [rejectOpen, setRejectOpen] = useState(false)
  const [rejectReason, setRejectReason] = useState('')
  const query = useQuery({ queryKey: ['order-drawer', orderId], queryFn: () => getOrder(orderId!), enabled: open && Boolean(orderId) })
  const data = query.data
  const mutation = useMutation({ mutationFn: () => confirmAndCreateWorkOrders(orderId!, data!.order.version), onSuccess: () => { message.success('已确认订单并生成工单'); client.invalidateQueries({ queryKey: ['order-drawer', orderId] }); client.invalidateQueries({ queryKey: ['orders'] }); client.invalidateQueries({ queryKey: ['work-orders'] }) }, onError: (e: Error) => message.error(e.message) })
  const reject = useMutation({ mutationFn: () => rejectOrder(orderId!, data!.order.version, rejectReason.trim()), onSuccess: () => { message.success('订单已打回客户'); setRejectOpen(false); setRejectReason(''); client.invalidateQueries({ queryKey: ['order-drawer', orderId] }); client.invalidateQueries({ queryKey: ['orders'] }) }, onError: (e: Error) => message.error(e.message) })
  useEffect(() => { if (!open) client.removeQueries({ queryKey: ['order-drawer', orderId] }) }, [open, orderId, client])
  const canOperate = data?.order.status === 'PENDING_CONFIRMATION'
  return <><Drawer title={data ? `订单 ${data.order.orderNo}` : '订单详情'} open={open} onClose={onClose} width={1100} extra={canOperate ? <Space><Button type="primary" loading={mutation.isPending} onClick={() => mutation.mutate()}>确认并生成工单</Button><Button danger loading={reject.isPending} onClick={() => setRejectOpen(true)}>打回</Button></Space> : null}>
    {query.isLoading ? <Spin /> : !data ? <Empty description="订单详情加载失败" /> : <OrderDetailContent data={data} />}
  </Drawer><Modal open={rejectOpen} title="打回订单" okText="确认打回" cancelText="取消" okButtonProps={{ danger: true }} confirmLoading={reject.isPending} onCancel={() => { setRejectOpen(false); setRejectReason('') }} onOk={() => { if (!rejectReason.trim()) { message.warning('请填写打回原因'); return }; reject.mutate() }} destroyOnHidden>
    <Typography.Paragraph>订单将退回客户，客户可以在订单详情中看到打回原因。</Typography.Paragraph>
    <Input.TextArea rows={4} maxLength={512} showCount value={rejectReason} onChange={e => setRejectReason(e.target.value)} placeholder="请输入打回原因（必填）" />
  </Modal></>
}
