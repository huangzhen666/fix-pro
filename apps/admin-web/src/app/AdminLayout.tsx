import { useMemo } from 'react'
import { Button, Layout, Menu, Space, Tag, Typography } from 'antd'
import { Outlet, useLocation, useNavigate } from 'react-router'
import { useAuthStore } from '../stores/authStore'
import { apiRequest } from '../api/http'

const { Header, Sider, Content } = Layout

const items = [
  { key: '/', label: '经营概览', permission: 'dashboard.view' },
  { key: '/catalog/skus', label: '维修 SKU', permission: 'catalog.sku.view' },
  { key: '/catalog/categories', label: '服务分类', permission: 'catalog.category.view' },
  { key: '/orders', label: '订单中心', permission: 'order.view' },
  { key: '/work-orders', label: '履约调度', permission: 'fulfillment.view' },
  { key: '/workers', label: '师傅管理', permission: 'worker.view' },
  { key: '/worker-skills', label: '工种与技能', permission: 'worker.skill.view' },
  { key: '/customers', label: '客户资产', permission: 'order.view' },
  { key: '/inventory', label: '材料库存', permission: 'catalog.sku.view' },
  { key: '/after-sales', label: '售后质保', permission: 'fulfillment.view' },
  { key: '/enterprises', label: '企业合同', permission: 'order.view' },
  { key: '/settings/users', label: '后台用户', permission: 'admin.user.view' },
  { key: '/settings/roles', label: '角色与权限', permission: 'admin.role.view' },
]

export function AdminLayout() {
  const location = useLocation()
  const navigate = useNavigate()
  const clearSession = useAuthStore((state) => state.clearSession)
  const permissions = useAuthStore((state) => state.permissions)
  const canSee = (permission: string) => permissions.includes(permission)
  const selectedKeys = useMemo(() => [location.pathname], [location.pathname])

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Sider width={220} theme="dark">
        <div className="brand-mark">
          <span className="brand-dot" />
          FixPro 运维中心
        </div>
        <Menu
          theme="dark"
          mode="inline"
          items={items.filter((item) => canSee(item.permission))}
          selectedKeys={selectedKeys}
          onClick={({ key }) => navigate(key)}
        />
      </Sider>
      <Layout>
        <Header
          style={{
            background: '#fff',
            padding: '0 24px',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            borderBottom: '1px solid #f0f0f0',
          }}
        >
          <Space>
            <Typography.Text strong>单城自营环境</Typography.Text>
            <Tag color="blue">V1 初始化</Tag>
          </Space>
          <Button
            type="text"
            onClick={async () => {
              try { await apiRequest('/api/v1/admin/auth/logout', { method: 'POST' }) } catch { /* session may already be expired */ }
              clearSession()
              navigate('/login')
            }}
          >
            退出登录
          </Button>
        </Header>
        <Content className="page-container">
          <Outlet />
        </Content>
      </Layout>
    </Layout>
  )
}
