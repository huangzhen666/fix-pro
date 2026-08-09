import { Button, Card, Form, Input, Space, Typography } from 'antd'
import { useNavigate } from 'react-router'
import { useAuthStore } from '../stores/authStore'

interface LoginValues {
  username: string
  password: string
}

export function LoginPage() {
  const navigate = useNavigate()
  const setBasicCredential = useAuthStore((state) => state.setBasicCredential)

  const submit = (values: LoginValues) => {
    setBasicCredential(values.username, values.password)
    navigate('/')
  }

  return (
    <main className="login-shell">
      <Card className="login-card" bordered={false}>
        <Space direction="vertical" size={4} style={{ marginBottom: 24 }}>
          <Typography.Title level={2} style={{ margin: 0 }}>
            FixPro 运维中心
          </Typography.Title>
          <Typography.Text type="secondary">
            当前为工程初始化阶段，使用后端 Bootstrap 管理员登录。
          </Typography.Text>
        </Space>
        <Form<LoginValues>
          layout="vertical"
          initialValues={{ username: 'admin', password: 'change-me-in-production' }}
          onFinish={submit}
        >
          <Form.Item label="用户名" name="username" rules={[{ required: true }]}>
            <Input autoComplete="username" />
          </Form.Item>
          <Form.Item label="密码" name="password" rules={[{ required: true }]}>
            <Input.Password autoComplete="current-password" />
          </Form.Item>
          <Button type="primary" htmlType="submit" block size="large">
            登录
          </Button>
        </Form>
      </Card>
    </main>
  )
}
