import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Alert, Button, Card, Col, Descriptions, Drawer, Form, Input, Row, Select, Space, Statistic, Table, Typography } from 'antd'
import { useNavigate } from 'react-router'
import { useAuthStore } from '../stores/authStore'
import { AuthMedia } from '../components/AuthMedia'
import { customerDetail, customerOrders, customerOverview, listCustomers, type Customer } from '../api/customers'
const at = (s?: string) => s ? new Date(s).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai' }) : '暂无'
const money = (v: number) => '¥' + (v / 100).toFixed(2)
export default function CustomerPage() {
  const permissions = useAuthStore(s => s.permissions)
  const user = useAuthStore(s => s.user)
  const scope = `${user?.orgId}:${user?.adminUserId}`
  const canView = permissions.includes('customer.view'), canOrders = permissions.includes('order.view')
  const navigate = useNavigate()
  const [filters, setFilters] = useState<Record<string, string>>({})
  const [page, setPage] = useState(1), [size, setSize] = useState(20)
  const [selected, setSelected] = useState(''), [orderPage, setOrderPage] = useState(1)
  const overview = useQuery({ queryKey: ['customer-overview', scope], queryFn: customerOverview, enabled: canView })
  const query = new URLSearchParams({ ...filters, page: String(page), pageSize: String(size) })
  const list = useQuery({ queryKey: ['customers', scope, query.toString()], queryFn: () => listCustomers(query), enabled: canView })
  const detail = useQuery({ queryKey: ['customer', scope, selected], queryFn: () => customerDetail(selected), enabled: canView && !!selected })
  const orders = useQuery({ queryKey: ['customer-orders', scope, selected, orderPage], queryFn: () => customerOrders(selected, orderPage), enabled: canView && canOrders && !!selected })
  if (!canView) return <Alert type="warning" message="没有用户查看权限" />
  const metrics = [['registered', '累计注册'], ['newToday', '今日新增'], ['new7Days', '近 7 天新增'], ['activeToday', '今日活跃'], ['active7Days', '近 7 天活跃'], ['active30Days', '近 30 天活跃']] as const
  const c = detail.data?.customer
  return <Space direction="vertical" size="large" style={{ width: '100%' }}>
    <Space><Typography.Title level={3} style={{ margin: 0 }}>用户管理</Typography.Title><Button onClick={() => { void overview.refetch(); void list.refetch(); if (selected) void detail.refetch() }}>刷新</Button></Space>
    {overview.error ? <Alert type="error" message={overview.error.message} /> : <Row gutter={[16, 16]}>{metrics.map(([k, label]) => <Col span={4} key={k}><Card loading={overview.isLoading}><Statistic title={label} value={overview.data?.[k] ?? '—'} /></Card></Col>)}</Row>}
    <Typography.Text type="secondary">总体统计不受列表筛选影响；北京时间自然日，活跃按最近一次有效业务交互计算。统计时间：{at(overview.data?.asOf)}</Typography.Text>
    <Card><Form layout="inline" onFinish={(values: Record<string, string>) => { setFilters(Object.fromEntries(Object.entries(values).filter(([, v]) => !!v).map(([k, v]) => [k, v.trim()]))); setPage(1) }}>
      <Form.Item name="customerId"><Input placeholder="用户编号" /></Form.Item><Form.Item name="name"><Input placeholder="昵称" /></Form.Item><Form.Item name="mobile"><Input placeholder="完整手机号精确查询" autoComplete="off" /></Form.Item>
      <Form.Item name="status"><Select allowClear placeholder="状态" style={{ width: 120 }} options={['ACTIVE', 'DISABLED', 'DELETED'].map((value, i) => ({ value, label: ['正常', '禁用', '已删除'][i] }))} /></Form.Item>
      <Form.Item name="hasOrders"><Select allowClear placeholder="订单使用" style={{ width: 120 }} options={[{ value: 'true', label: '有订单' }, { value: 'false', label: '无订单' }]} /></Form.Item>
      <Form.Item name="registeredFrom"><Input type="date" aria-label="注册起始日期" /></Form.Item><Form.Item name="registeredTo"><Input type="date" aria-label="注册结束日期" /></Form.Item>
      <Form.Item name="sort"><Select placeholder="排序" style={{ width: 140 }} options={[{ value: 'registered', label: '最近注册' }, { value: 'active', label: '最近活跃' }, { value: 'orders', label: '订单最多' }]} /></Form.Item><Button htmlType="submit" type="primary">查询</Button>
    </Form></Card>
    {list.error && <Alert type="error" message={list.error.message} />}
    <Table<Customer> rowKey="id" loading={list.isLoading} dataSource={list.data?.items ?? []} scroll={{ x: 1200 }} pagination={{ current: page, pageSize: size, total: list.data?.total ?? 0, showSizeChanger: true, showTotal: n => `筛选结果 ${n} 位用户`, onChange: (p, s) => { setPage(p); setSize(s) } }} columns={[
      { title: '编号', dataIndex: 'id' }, { title: '用户', render: (_, r) => <Space>{r.avatarUrl && <AuthMedia url={r.avatarUrl} type="IMAGE" name="头像" />}{r.displayName}</Space> },
      { title: '手机号', dataIndex: 'mobileMasked' }, { title: '状态', dataIndex: 'status' }, { title: '注册时间', dataIndex: 'createdAt', render: at }, { title: '最近登录', dataIndex: 'lastLoginAt', render: at }, { title: '最近活跃', dataIndex: 'lastActivityAt', render: at },
      { title: '订单 / 完成 / 取消', render: (_, r) => `${r.usage.orderCount} / ${r.usage.completedCount} / ${r.usage.cancelledCount}` }, { title: '累计实付', render: (_, r) => money(r.usage.paidAmount) }, { title: '最近下单', render: (_, r) => at(r.usage.lastOrderAt) },
      { title: '操作', render: (_, r) => <Button onClick={() => { setSelected(r.id); setOrderPage(1) }}>详情</Button> },
    ]} />
    <Drawer title="用户使用详情" width={800} open={!!selected} onClose={() => setSelected('')}>
      {detail.error && <Alert type="error" message={detail.error.message} />}
      {c && <Descriptions bordered column={2} items={[{ key: 'id', label: '编号', children: c.id }, { key: 'name', label: '昵称', children: c.displayName }, { key: 'phone', label: '手机号', children: c.mobileMasked }, { key: 'status', label: '状态', children: c.status }, { key: 'addr', label: '地址数', children: detail.data?.addressCount }, { key: 'cart', label: '购物车服务项', children: detail.data?.cartItemCount }, { key: 'orders', label: '订单数', children: c.usage.orderCount }, { key: 'paid', label: '累计实付', children: money(c.usage.paidAmount) }, { key: 'registered', label: '注册时间', children: at(c.createdAt) }, { key: 'login', label: '最近登录', children: at(c.lastLoginAt) }, { key: 'activity', label: '最近活跃', children: at(c.lastActivityAt) }, { key: 'complete', label: '完成 / 取消', children: `${c.usage.completedCount} / ${c.usage.cancelledCount}` }, { key: 'lastOrder', label: '最近下单', children: at(c.usage.lastOrderAt) }]} />}
      {canOrders ? <><Typography.Title level={5}>最近订单</Typography.Title>{orders.error && <Alert type="error" message={orders.error.message} />}<Table rowKey="id" loading={orders.isLoading} dataSource={orders.data?.items ?? []} pagination={{ current: orderPage, pageSize: 10, total: orders.data?.total ?? 0, onChange: setOrderPage }} columns={[{ title: '订单号', dataIndex: 'orderNo', render: (no: string, r) => <Button type="link" onClick={() => navigate('/orders/' + r.id)}>{no}</Button> }, { title: '状态', dataIndex: 'status' }, { title: '实付', dataIndex: 'paidAmount', render: money }, { title: '创建时间', dataIndex: 'createdAt', render: at }]} /></> : <Typography.Paragraph>没有订单查看权限，仅展示使用汇总。</Typography.Paragraph>}
    </Drawer>
  </Space>
}
