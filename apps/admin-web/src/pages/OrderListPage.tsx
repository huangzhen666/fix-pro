import { useState } from 'react'
import { Button, Card, DatePicker, Input, Select, Space, Table, Tag, Typography, message } from 'antd'
import type { Dayjs } from 'dayjs'
import dayjs from 'dayjs'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { confirmAndCreateWorkOrders, listOrders } from '../api/orders'
import { orderStatusLabel } from '../utils/enums'
import { OrderDetailDrawer } from '../components/OrderDetailDrawer'

const { RangePicker } = DatePicker
const fmt = (v?: Dayjs) => v ? v.format('YYYY-MM-DD') : ''
export default function OrderListPage() {
  const [keyword, setKeyword] = useState(''), [contact, setContact] = useState(''), [status, setStatus] = useState(''), [range, setRange] = useState<[Dayjs | null, Dayjs | null]>([null, null]), [page, setPage] = useState(1)
  const client = useQueryClient(), from = fmt(range[0] || undefined), to = fmt(range[1] || undefined)
  const [drawerOrderId, setDrawerOrderId] = useState<string>()
  const query = useQuery({ queryKey: ['orders', keyword, contact, status, from, to, page], queryFn: () => listOrders(keyword, status, contact, from, to, page) })
  const confirm = useMutation({ mutationFn: (row: any) => confirmAndCreateWorkOrders(row.id, row.version), onSuccess: () => { message.success('已确认订单并生成工单'); client.invalidateQueries({ queryKey: ['orders'] }) }, onError: (e: Error) => message.error(e.message) })
  function quick(days: number) { const end = dayjs(); setRange([end.subtract(days - 1, 'day'), end]); setPage(1) }
  return <Card><Space direction="vertical" size="large" style={{ width: '100%' }}>
    <Typography.Title level={3} style={{ margin: 0 }}>订单中心</Typography.Title>
    <Space wrap><Input.Search allowClear placeholder="订单号、联系人、手机号" style={{ width: 300 }} onSearch={v => { setPage(1); setKeyword(v) }} /><Input.Search allowClear placeholder="联系人/手机号" style={{ width: 180 }} onSearch={v => { setPage(1); setContact(v) }} /><Select value={status} onChange={v => { setPage(1); setStatus(v) }} style={{ width: 160 }} options={[{ value: '', label: '全部订单状态' }, ...['PENDING_CONFIRMATION', 'FULFILLING', 'WAITING_ACCEPTANCE', 'COMPLETED', 'CANCELLED'].map(v => ({ value: v, label: orderStatusLabel(v) }))]} /><RangePicker value={range} onChange={v => { setPage(1); setRange(v as [Dayjs | null, Dayjs | null]) }} /><Button onClick={() => quick(1)}>今天</Button><Button onClick={() => quick(7)}>近一周</Button><Button onClick={() => quick(30)}>近一个月</Button></Space>
    <Table rowKey="id" loading={query.isLoading} dataSource={query.data?.items} pagination={{ current: page, pageSize: 20, total: query.data?.total, showSizeChanger: false, onChange: p => setPage(p) }} onRow={r => ({ onClick: () => setDrawerOrderId(r.id), style: { cursor: 'pointer' } })} columns={[{ title: '订单号', dataIndex: 'orderNo' }, { title: '状态', dataIndex: 'status', render: v => <Tag color="gold">{orderStatusLabel(v)}</Tag> }, { title: '联系人', dataIndex: 'contactName' }, { title: '手机号', dataIndex: 'contactMobile' }, { title: '项目数', dataIndex: 'itemCount' }, { title: '总金额', dataIndex: 'totalAmount', render: v => `¥${(v / 100).toFixed(2)}` }, { title: '创建时间', dataIndex: 'createdAt', render: v => new Date(v).toLocaleString() }, { title: '操作', render: (_: unknown, row: any) => row.status === 'PENDING_CONFIRMATION' ? <Space><Button type="link" loading={confirm.isPending} onClick={e => { e.stopPropagation(); confirm.mutate(row) }}>确认并生成工单</Button><Button type="link" onClick={e => { e.stopPropagation(); setDrawerOrderId(row.id) }}>查看详情</Button></Space> : <Button type="link" onClick={e => { e.stopPropagation(); setDrawerOrderId(row.id) }}>查看详情</Button> }]} />
    <OrderDetailDrawer open={Boolean(drawerOrderId)} orderId={drawerOrderId} onClose={() => setDrawerOrderId(undefined)} />
  </Space></Card>
}
