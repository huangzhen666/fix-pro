import { useEffect } from 'react'
import { Button, Card, Descriptions, Divider, Drawer, Empty, Space, Spin, Tag, message } from 'antd'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { confirmAndCreateWorkOrders, getOrder } from '../api/orders'
import { AuthMedia } from './AuthMedia'
import { orderStatusLabel } from '../utils/enums'

function formatAppointment(date?: string, slot?: string) {
  if (!date) return '未设置'
  const hour = slot ? Number(slot.slice(0, 2)) : NaN
  const range = Number.isFinite(hour) ? `${slot}-${String(hour + 2).padStart(2, '0')}:00` : (slot || '')
  return `${new Date(date).toLocaleDateString()} ${range}`.trim()
}

export function OrderDetailContent({ data }: { data: Awaited<ReturnType<typeof getOrder>> }) {
  return <Space direction="vertical" size="middle" style={{ width: '100%' }}><Descriptions column={2} bordered size="small" items={[{ key: 'status', label: '状态', children: <Tag color="gold">{orderStatusLabel(data.order.status)}</Tag> }, { key: 'amount', label: '总金额', children: `¥${(data.order.totalAmount / 100).toFixed(2)}` }, { key: 'name', label: '联系人', children: data.order.contactName }, { key: 'mobile', label: '手机号', children: data.order.contactMobile }, { key: 'address', label: '服务地址', span: 2, children: data.order.serviceAddress }, { key: 'appointment', label: '客户预约', span: 2, children: formatAppointment(data.order.appointmentAt, data.order.appointmentSlot) }, { key: 'created', label: '下单时间', children: new Date(data.order.createdAt).toLocaleString() }]} />{data.items.map(item => <Card key={item.id} size="small" title={`${item.skuName} · V${item.skuVersion}`}><Descriptions column={1} size="small" items={[{ key: 'code', label: 'SKU 编码', children: item.skuCode }, { key: 'scope', label: '服务范围', children: item.serviceScope }, { key: 'exclusions', label: '除外项', children: item.exclusions }, { key: 'warranty', label: '售后/质保', children: item.warrantyDescription }, { key: 'fault', label: '故障描述', children: item.faultDescription || '未填写' }, { key: 'price', label: '成交金额', children: `¥${(item.unitPrice / 100).toFixed(2)} × ${item.quantity}${item.unit} = ¥${(item.subtotal / 100).toFixed(2)}` }]} /><Divider>客户故障资料（{item.faultMedia.length}）</Divider><Space wrap>{item.faultMedia.map(m => <AuthMedia key={m.id} url={m.url} type={m.mediaType} name={m.name} />)}</Space></Card>)}</Space>
}

export function OrderDetailDrawer({ open, orderId, onClose }: { open: boolean; orderId?: string; onClose: () => void }) {
  const client = useQueryClient()
  const query = useQuery({ queryKey: ['order-drawer', orderId], queryFn: () => getOrder(orderId!), enabled: open && Boolean(orderId) })
  const data = query.data
  const mutation = useMutation({ mutationFn: () => confirmAndCreateWorkOrders(orderId!, data!.order.version), onSuccess: () => { message.success('已确认订单并生成工单'); client.invalidateQueries({ queryKey: ['order-drawer', orderId] }); client.invalidateQueries({ queryKey: ['orders'] }); client.invalidateQueries({ queryKey: ['work-orders'] }) }, onError: (e: Error) => message.error(e.message) })
  useEffect(() => { if (!open) client.removeQueries({ queryKey: ['order-drawer', orderId] }) }, [open, orderId, client])
  return <Drawer title={data ? `订单 ${data.order.orderNo}` : '订单详情'} open={open} onClose={onClose} width={1100} extra={data?.order.status === 'PENDING_CONFIRMATION' ? <Button type="primary" loading={mutation.isPending} onClick={() => mutation.mutate()}>确认并生成工单</Button> : null}>
    {query.isLoading ? <Spin /> : !data ? <Empty description="订单详情加载失败" /> : <OrderDetailContent data={data} />}
  </Drawer>
}
