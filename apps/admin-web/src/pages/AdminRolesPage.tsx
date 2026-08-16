import { useState } from 'react'
import { App, Button, Card, Checkbox, Divider, Form, Input, Modal, Popconfirm, Space, Table, Tag, Typography } from 'antd'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { apiRequest } from '../api/http'

type Permission = { code: string; resource: string; action: string; type: string; sortOrder?: number }
type Role = { id: number; name: string; code: string; description?: string; status: string; isBuiltin: boolean; permissionCodes?: string[]; version: number }

const resourceLabels: Record<string, string> = {
  dashboard: '经营概览',
  'catalog.sku': '维修 SKU',
  'catalog.category': '服务分类',
  order: '订单中心',
  fulfillment: '履约调度',
  worker: '师傅管理',
  'worker.skill': '工种与技能',
  media: '媒体资料',
  'admin.user': '后台用户',
  'admin.role': '角色与权限',
  audit: '审计日志',
}

const actionLabels: Record<string, string> = {
  view: '查看',
  create: '新增',
  update: '编辑',
  publish: '上架',
  manage: '管理',
  confirm: '确认并生成工单',
  dispatch: '派单',
  reassign: '改派',
  reschedule: '调整预约时间',
  qa_review: '审核员初审',
  director_review: '总监审核',
  customer_service: '客服确认',
  disable: '禁用',
  reset_password: '重置密码',
  assign_permission: '分配权限',
  delete: '删除',
}

function permissionName(permission: Permission) {
  return resourceLabels[permission.resource] ?? permission.resource
}

function actionName(permission: Permission) {
  return actionLabels[permission.action] ?? permission.action
}

export default function AdminRolesPage() {
  const { message } = App.useApp()
  const qc = useQueryClient()
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<Role | undefined>()
  const [selectedPermissions, setSelectedPermissions] = useState<string[]>([])
  const [form] = Form.useForm()
  const roles = useQuery({ queryKey: ['admin-roles'], queryFn: () => apiRequest<Role[]>('/api/v1/admin/roles') })
  const permissions = useQuery({ queryKey: ['admin-permissions'], queryFn: () => apiRequest<Permission[]>('/api/v1/admin/permissions') })

  const menuPermissions = (permissions.data ?? []).filter((permission) => permission.type === 'MENU').sort((a, b) => (a.sortOrder ?? 0) - (b.sortOrder ?? 0))
  const actionPermissions = (permissions.data ?? []).filter((permission) => permission.type === 'ACTION')
  const permissionGroups = menuPermissions.map((menu) => ({ menu, actions: actionPermissions.filter((action) => action.resource === menu.resource).sort((a, b) => (a.sortOrder ?? 0) - (b.sortOrder ?? 0)) }))
  const orphanActions = actionPermissions.filter((action) => !menuPermissions.some((menu) => menu.resource === action.resource)).sort((a, b) => (a.sortOrder ?? 0) - (b.sortOrder ?? 0))

  function closeEditor() {
    setOpen(false)
    setEditing(undefined)
    setSelectedPermissions([])
    form.resetFields()
  }

  function setPermission(code: string, checked: boolean) {
    setSelectedPermissions((current) => checked ? [...new Set([...current, code])] : current.filter((item) => item !== code))
  }

  function setMenuPermission(menu: Permission, checked: boolean) {
    const relatedCodes = actionPermissions.filter((action) => action.resource === menu.resource).map((action) => action.code)
    setSelectedPermissions((current) => {
      if (checked) return [...new Set([...current, menu.code, ...relatedCodes])]
      const removeCodes = new Set([menu.code, ...relatedCodes])
      return current.filter((code) => !removeCodes.has(code))
    })
  }

  async function save() {
    try { const values = await form.validateFields(); await apiRequest(editing ? `/api/v1/admin/roles/${editing.id}` : '/api/v1/admin/roles', { method: editing ? 'PUT' : 'POST', body: JSON.stringify({ ...values, permissionCodes: selectedPermissions, version: editing?.version ?? 0 }) }); message.success(editing ? '角色已更新' : '角色已创建'); closeEditor(); qc.invalidateQueries({ queryKey: ['admin-roles'] }) }
    catch (e) { if (e instanceof Error) message.error(e.message) }
  }
  async function edit(row: Role) {
    try { const detail = await apiRequest<Role & { permissionCodes: string[] }>(`/api/v1/admin/roles/${row.id}`); setEditing(detail); setSelectedPermissions(detail.permissionCodes ?? []); form.setFieldsValue(detail); setOpen(true) } catch (e) { if (e instanceof Error) message.error(e.message) }
  }
  return <Card>
    <Space direction="vertical" style={{ width: '100%' }} size="large">
      <Space><Typography.Title level={3} style={{ margin: 0 }}>角色与权限</Typography.Title><Button type="primary" onClick={() => { setEditing(undefined); setSelectedPermissions([]); form.resetFields(); setOpen(true) }}>新增角色</Button></Space>
      <Table rowKey="id" loading={roles.isLoading} dataSource={roles.data} columns={[
        { title: '角色名称', dataIndex: 'name' }, { title: '编码', dataIndex: 'code' }, { title: '权限数', dataIndex: 'permissionCodes', render: (v: string[] = []) => v.length },
        { title: '状态', dataIndex: 'status', render: (v: string) => <Tag color={v === 'ACTIVE' ? 'green' : 'default'}>{v === 'ACTIVE' ? '启用' : '禁用'}</Tag> },
        { title: '类型', dataIndex: 'isBuiltin', render: (v: boolean) => v ? <Tag color="blue">内置</Tag> : <Tag>自定义</Tag> },
        { title: '操作', render: (_: unknown, row: Role) => <Space><Button type="link" disabled={row.isBuiltin} onClick={() => void edit(row)}>编辑</Button><Popconfirm title="确认删除角色？" description="已被用户使用的角色不能删除。" okText="确认删除" cancelText="取消" onConfirm={async () => { try { await apiRequest(`/api/v1/admin/roles/${row.id}`, { method: 'DELETE' }); message.success('角色已删除'); qc.invalidateQueries({ queryKey: ['admin-roles'] }) } catch (e) { if (e instanceof Error) message.error(e.message) } }}><Button type="link" danger disabled={row.isBuiltin}>删除</Button></Popconfirm></Space> },
      ]} />
    </Space>
    <Modal open={open} title={editing ? '编辑角色' : '新增角色'} width={920} styles={{ body: { maxHeight: '65vh', overflowY: 'auto', paddingRight: 8 } }} onCancel={closeEditor} onOk={() => void save()} okText="保存">
      <Form form={form} layout="vertical">
        <Form.Item name="name" label="角色名称" rules={[{ required: true, min: 2 }]}><Input /></Form.Item>
        <Form.Item name="description" label="描述"><Input.TextArea maxLength={255} /></Form.Item>
        <Form.Item label="权限配置" extra="勾选菜单会自动勾选该页面下的全部操作；如需精细控制，可在菜单展开后取消单个操作。">
          <Space direction="vertical" style={{ width: '100%' }} size="middle">
            {permissionGroups.map(({ menu, actions }) => {
              const selectedActionCount = actions.filter((action) => selectedPermissions.includes(action.code)).length
              return <Card key={menu.code} size="small" styles={{ body: { padding: '12px 16px' } }}>
                <Space direction="vertical" style={{ width: '100%' }} size={8}>
                  <Space align="start">
                    <Checkbox checked={selectedPermissions.includes(menu.code)} onChange={(event) => setMenuPermission(menu, event.target.checked)} />
                    <Space direction="vertical" size={0}>
                      <Space><Tag color="blue">菜单</Tag><Typography.Text strong>{permissionName(menu)}</Typography.Text><Typography.Text type="secondary">{menu.code}</Typography.Text></Space>
                      <Typography.Text type="secondary">页面入口</Typography.Text>
                    </Space>
                  </Space>
                  {actions.length > 0 && <><Divider style={{ margin: '4px 0' }} /><Space direction="vertical" style={{ width: '100%' }} size={6}>
                    <Space><Typography.Text type="secondary">关联操作</Typography.Text><Tag>{selectedActionCount}/{actions.length} 已选择</Tag></Space>
                    <Space wrap style={{ paddingLeft: 32 }}>
                      {actions.map((action) => <Checkbox key={action.code} checked={selectedPermissions.includes(action.code)} onChange={(event) => setPermission(action.code, event.target.checked)}><Space size={4}><Typography.Text>{actionName(action)}</Typography.Text><Typography.Text type="secondary">({action.code})</Typography.Text></Space></Checkbox>)}
                    </Space>
                  </Space></>}
                </Space>
              </Card>
            })}
            {orphanActions.length > 0 && <Card size="small" title="其他操作权限" styles={{ body: { padding: '12px 16px' } }}>
              <Space wrap>
                {orphanActions.map((action) => <Checkbox key={action.code} checked={selectedPermissions.includes(action.code)} onChange={(event) => setPermission(action.code, event.target.checked)}><Space size={4}><Typography.Text>{permissionName(action)} · {actionName(action)}</Typography.Text><Typography.Text type="secondary">({action.code})</Typography.Text></Space></Checkbox>)}
              </Space>
            </Card>}
          </Space>
        </Form.Item>
      </Form>
    </Modal>
  </Card>
}
