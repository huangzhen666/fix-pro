import { useState } from 'react'
import { Button, Card, DatePicker, Input, Modal, Select, Space, Table, Tag, Typography, message } from 'antd'
import type { Dayjs } from 'dayjs'
import dayjs from 'dayjs'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { confirmAndCreateWorkOrders, listOrders, rejectOrder } from '../api/orders'
import { orderStatusLabel } from '../utils/enums'
import { OrderDetailDrawer } from '../components/OrderDetailDrawer'

const { RangePicker } = DatePicker
const fmt = (v?: Dayjs) => v ? v.format('YYYY-MM-DD') : ''
const formatDateTime = (value: string) => {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString([], { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
}
export default function OrderListPage() {
  const [keyword, setKeyword] = useState(''), [contact, setContact] = useState(''), [status, setStatus] = useState(''), [range, setRange] = useState<[Dayjs | null, Dayjs | null]>([null, null]), [page, setPage] = useState(1)
  const client = useQueryClient(), from = fmt(range[0] || undefined), to = fmt(range[1] || undefined)
  const [drawerOrderId, setDrawerOrderId] = useState<string>()
  const [rejecting, setRejecting] = useState<any>()
  const [rejectReason, setRejectReason] = useState('')
  const query = useQuery({ queryKey: ['orders', keyword, contact, status, from, to, page], queryFn: () => listOrders(keyword, status, contact, from, to, page) })
  const confirm = useMutation({ mutationFn: (row: any) => confirmAndCreateWorkOrders(row.id, row.version), onSuccess: () => { message.success('已确认订单并生成工单'); client.invalidateQueries({ queryKey: ['orders'] }) }, onError: (e: Error) => message.error(e.message) })
  const reject = useMutation({ mutationFn: (input: { row: any; reason: string }) => rejectOrder(input.row.id, input.row.version, input.reason), onSuccess: () => { message.success('订单已打回客户'); setRejecting(undefined); setRejectReason(''); client.invalidateQueries({ queryKey: ['orders'] }) }, onError: (e: Error) => message.error(e.message) })
  function quick(days: number) { const end = dayjs(); setRange([end.subtract(days - 1, 'day'), end]); setPage(1) }
  function openReject(row: any) { setRejecting(row); setRejectReason('') }
  function submitReject() { const reason = rejectReason.trim(); if (!reason) { message.warning('请填写打回原因'); return }; reject.mutate({ row: rejecting, reason }) }
  return <Card className="order-list-page" styles={{ body: { padding: '16px 18px' } }}><Space className="order-page-content" direction="vertical" size={14} style={{ width: '100%' }}>
    <Typography.Title level={3} style={{ margin: 0, fontSize: 26 }}>订单中心</Typography.Title>
    <Space className="order-filters" wrap size={[8, 8]}>
      <Input.Search allowClear placeholder="订单号、联系人、手机号" style={{ width: 280 }} onSearch={v => { setPage(1); setKeyword(v) }} />
      <Input.Search allowClear placeholder="联系人/手机号" style={{ width: 170 }} onSearch={v => { setPage(1); setContact(v) }} />
      <Select value={status} onChange={v => { setPage(1); setStatus(v) }} style={{ width: 150 }} options={[{ value: '', label: '全部订单状态' }, ...['PENDING_CONFIRMATION', 'FULFILLING', 'WAITING_ACCEPTANCE', 'COMPLETED', 'CANCELLED'].map(v => ({ value: v, label: orderStatusLabel(v) }))]} />
      <RangePicker value={range} onChange={v => { setPage(1); setRange(v as [Dayjs | null, Dayjs | null]) }} />
      <Button onClick={() => quick(1)}>今天</Button><Button onClick={() => quick(7)}>近一周</Button><Button onClick={() => quick(30)}>近一个月</Button>
    </Space>
    <Table className="order-table" size="small" tableLayout="fixed" rowKey="id" loading={query.isLoading} dataSource={query.data?.items} pagination={{ current: page, pageSize: 20, total: query.data?.total, showSizeChanger: false, onChange: p => setPage(p) }} onRow={r => ({ onClick: () => setDrawerOrderId(r.id), style: { cursor: 'pointer' } })} columns={[{ title: '订单号', dataIndex: 'orderNo', width: 180, ellipsis: true }, { title: '状态', dataIndex: 'status', width: 78, render: (v, row) => <Tag color="gold">{v === 'CANCELLED' && row.cancelReason ? '商家已打回' : orderStatusLabel(v)}</Tag> }, { title: '联系人', dataIndex: 'contactName', width: 62, ellipsis: true }, { title: '手机号', dataIndex: 'contactMobile', width: 105 }, { title: '项目数', dataIndex: 'itemCount', width: 60 }, { title: '总金额', dataIndex: 'totalAmount', width: 72, render: v => `¥${(v / 100).toFixed(2)}` }, { title: '创建时间', dataIndex: 'createdAt', width: 120, render: v => <span className="order-date">{formatDateTime(v)}</span> }, { title: '操作', width: 230, render: (_: unknown, row: any) => row.status === 'PENDING_CONFIRMATION' ? <Space className="order-action-space" size={0}><Button type="link" loading={confirm.isPending} onClick={e => { e.stopPropagation(); confirm.mutate(row) }}>确认并生成工单</Button><Button type="link" danger loading={reject.isPending && rejecting?.id === row.id} onClick={e => { e.stopPropagation(); openReject(row) }}>打回</Button><Button type="link" onClick={e => { e.stopPropagation(); setDrawerOrderId(row.id) }}>查看详情</Button></Space> : <Button type="link" onClick={e => { e.stopPropagation(); setDrawerOrderId(row.id) }}>查看详情</Button> }]} />
    <Modal open={Boolean(rejecting)} title="打回订单" okText="确认打回" cancelText="取消" okButtonProps={{ danger: true }} confirmLoading={reject.isPending} onCancel={() => { setRejecting(undefined); setRejectReason('') }} onOk={submitReject} destroyOnHidden>
      <Typography.Paragraph>订单将退回客户，客户可以在订单详情中看到打回原因。</Typography.Paragraph>
      <Input.TextArea rows={4} maxLength={512} showCount value={rejectReason} onChange={e => setRejectReason(e.target.value)} placeholder="请输入打回原因（必填）" />
    </Modal>
    <OrderDetailDrawer open={Boolean(drawerOrderId)} orderId={drawerOrderId} onClose={() => setDrawerOrderId(undefined)} />
  </Space></Card>
}
