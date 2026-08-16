import { useState } from 'react'
import { App, Button, Card, Form, Input, Modal, Popconfirm, Select, Space, Table, Tag, Typography } from 'antd'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { apiRequest } from '../api/http'

type Role = { id: number; name: string; status: string }
type AdminUser = { id: number; username: string; displayName: string; status: string; mustChangePassword: boolean; version: number; roleNames?: string[]; roleCodes?: string[]; createdAt: string }
type TemporaryCredential = { username: string; password: string }

function formatDateTime(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  return new Intl.DateTimeFormat('zh-CN', {
    timeZone: 'Asia/Shanghai',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hourCycle: 'h23',
  }).format(date).replaceAll('/', '-')
}

export default function AdminUsersPage() {
  const { message } = App.useApp()
  const qc = useQueryClient()
  const [open, setOpen] = useState(false)
  const [temporaryCredential, setTemporaryCredential] = useState<TemporaryCredential>()
  const [form] = Form.useForm()
  const users = useQuery({ queryKey: ['admin-users'], queryFn: () => apiRequest<AdminUser[]>('/api/v1/admin/users') })
  const roles = useQuery({ queryKey: ['admin-roles'], queryFn: () => apiRequest<Role[]>('/api/v1/admin/roles') })
  async function create() {
    try {
      const values = await form.validateFields()
      const result = await apiRequest<{ id: number; temporaryPassword: string }>('/api/v1/admin/users', { method: 'POST', body: JSON.stringify(values) })
      setTemporaryCredential({ username: values.username, password: result.temporaryPassword })
      setOpen(false); form.resetFields(); qc.invalidateQueries({ queryKey: ['admin-users'] })
    } catch (e) { if (e instanceof Error) message.error(e.message) }
  }

  async function resetPassword(row: AdminUser) {
    try {
      const result = await apiRequest<{ temporaryPassword: string }>(`/api/v1/admin/users/${row.id}/reset-password`, { method: 'POST' })
      setTemporaryCredential({ username: row.username, password: result.temporaryPassword })
      qc.invalidateQueries({ queryKey: ['admin-users'] })
    } catch (e) { if (e instanceof Error) message.error(e.message) }
  }

  async function copyTemporaryPassword() {
    if (!temporaryCredential) return
    if (!navigator.clipboard) {
      message.warning('浏览器不支持自动复制，请手动复制临时密码')
      return
    }
    try {
      await navigator.clipboard.writeText(temporaryCredential.password)
      message.success('临时密码已复制')
    } catch {
      message.warning('复制失败，请手动复制临时密码')
    }
  }
  return <Card>
    <Space direction="vertical" style={{ width: '100%' }} size="large">
      <Space><Typography.Title level={3} style={{ margin: 0 }}>后台用户</Typography.Title><Button type="primary" onClick={() => setOpen(true)}>新增用户</Button></Space>
      <Table rowKey="id" loading={users.isLoading} dataSource={users.data} columns={[
        { title: '用户名', dataIndex: 'username' }, { title: '显示名', dataIndex: 'displayName' },
        { title: '角色', dataIndex: 'roleNames', render: (roleNames: string[] = []) => roleNames.length > 0 ? <Space wrap size={[4, 4]}>{roleNames.map((name) => <Tag key={name} color="blue">{name}</Tag>)}</Space> : <Typography.Text type="secondary">未分配</Typography.Text> },
        { title: '状态', dataIndex: 'status', render: (v: string) => <Tag color={v === 'ACTIVE' ? 'green' : 'default'}>{v === 'ACTIVE' ? '启用' : v === 'LOCKED' ? '锁定' : '禁用'}</Tag> },
        { title: '密码状态', dataIndex: 'mustChangePassword', render: (v: boolean) => v ? <Tag color="orange">首次登录需改密</Tag> : <Tag color="green">正常</Tag> },
        { title: '创建时间', dataIndex: 'createdAt', render: (value: string) => formatDateTime(value) },
        { title: '操作', render: (_: unknown, row: AdminUser) => <Space><Popconfirm title="确认重置密码？" description="原密码和已有登录会话将立即失效，并生成一次性临时密码。" okText="确认重置" cancelText="取消" onConfirm={() => void resetPassword(row)}><Button type="link">重置密码</Button></Popconfirm><Popconfirm title={row.status === 'ACTIVE' ? '确认禁用用户？' : '确认启用用户？'} okText="确认" cancelText="取消" onConfirm={async () => { try { await apiRequest(`/api/v1/admin/users/${row.id}/status`, { method: 'POST', body: JSON.stringify({ status: row.status === 'ACTIVE' ? 'DISABLED' : 'ACTIVE', version: row.version }) }); message.success('状态已更新'); qc.invalidateQueries({ queryKey: ['admin-users'] }) } catch (e) { if (e instanceof Error) message.error(e.message) } }}><Button type="link" danger={row.status === 'ACTIVE'}>{row.status === 'ACTIVE' ? '禁用' : '启用'}</Button></Popconfirm></Space> },
      ]} />
    </Space>
    <Modal open={open} title="新增后台用户" onCancel={() => setOpen(false)} onOk={() => void create()} okText="创建">
      <Form form={form} layout="vertical">
        <Form.Item name="username" label="用户名" rules={[{ required: true, min: 3, max: 64 }]}><Input autoComplete="off" /></Form.Item>
        <Form.Item name="displayName" label="显示名" rules={[{ required: true, min: 2, max: 64 }]}><Input /></Form.Item>
        <Form.Item name="roleIds" label="角色"><Select mode="multiple" options={roles.data?.filter((x) => x.status === 'ACTIVE').map((x) => ({ value: x.id, label: x.name }))} /></Form.Item>
      </Form>
    </Modal>
    <Modal open={Boolean(temporaryCredential)} title="临时密码已生成" onCancel={() => setTemporaryCredential(undefined)} footer={<Button type="primary" onClick={() => setTemporaryCredential(undefined)}>我已保存</Button>} destroyOnHidden>
      <Space direction="vertical" style={{ width: '100%' }} size="middle">
        <Typography.Paragraph>用户 <Typography.Text strong>{temporaryCredential?.username}</Typography.Text> 的原密码已经失效，现有登录会话也已退出。</Typography.Paragraph>
        <Input.Password value={temporaryCredential?.password} readOnly addonAfter={<Button type="link" onClick={() => void copyTemporaryPassword()}>复制</Button>} />
        <Typography.Paragraph type="warning" style={{ marginBottom: 0 }}>请将临时密码安全地交给用户。用户首次登录后必须立即修改密码，此临时密码只显示这一次。</Typography.Paragraph>
      </Space>
    </Modal>
  </Card>
}
