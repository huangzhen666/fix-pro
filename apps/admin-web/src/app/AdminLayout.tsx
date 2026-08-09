import { useMemo } from 'react'
import { Button, Layout, Menu, Space, Tag, Typography } from 'antd'
import { Outlet, useLocation, useNavigate } from 'react-router'
import { useAuthStore } from '../stores/authStore'

const { Header, Sider, Content } = Layout

const items = [
  { key: '/', label: '经营概览' },
  { key: '/catalog/skus', label: '维修 SKU' },
  { key: '/catalog/categories', label: '服务分类' },
  { key: '/orders', label: '订单中心' },
  { key: '/work-orders', label: '履约调度' },
  { key: '/customers', label: '客户资产' },
  { key: '/inventory', label: '材料库存' },
  { key: '/after-sales', label: '售后质保' },
  { key: '/enterprises', label: '企业合同' },
  { key: '/settings', label: '系统设置' },
]

export function AdminLayout() {
  const location = useLocation()
  const navigate = useNavigate()
  const clearCredential = useAuthStore((state) => state.clearCredential)
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
          items={items}
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
            onClick={() => {
              clearCredential()
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
