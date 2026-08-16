import { useState } from 'react'
import { Alert, Button, Card, Form, Input, Space, Typography } from 'antd'
import { useNavigate } from 'react-router'
import { useAuthStore } from '../stores/authStore'
import { ApiError, apiRequest } from '../api/http'

interface LoginValues {
  username: string
  password: string
}

export function LoginPage() {
  const navigate = useNavigate()
  const setSession = useAuthStore((state) => state.setSession)
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)

  const submit = async (values: LoginValues) => {
    setError('')
    setSubmitting(true)
    try {
      const result = await apiRequest<{ user: Parameters<typeof setSession>[0]; mustChangePassword: boolean }>('/api/v1/admin/auth/login', {
        method: 'POST',
        body: JSON.stringify({ orgId: 1, username: values.username, password: values.password }),
      })
      const me = await apiRequest<{ user: Parameters<typeof setSession>[0]; permissions: Array<{ code: string }> }>('/api/v1/admin/auth/me')
      const user = me.user as NonNullable<Parameters<typeof setSession>[0]>
      setSession({ ...user, mustChangePassword: result.mustChangePassword }, me.permissions.map((item) => item.code))
      navigate(result.mustChangePassword ? '/settings/change-password' : '/')
    } catch (e) {
      if (e instanceof ApiError && e.code === 'UNAUTHORIZED') setError('用户名或密码错误，请检查后重试。')
      else if (e instanceof Error) setError(e.message)
      else setError('登录失败，请稍后重试。')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <main className="login-shell">
      <Card className="login-card" bordered={false}>
        <Space direction="vertical" size={4} style={{ marginBottom: 24 }}>
          <Typography.Title level={2} style={{ margin: 0 }}>
            FixPro 运维中心
          </Typography.Title>
          <Typography.Text type="secondary">
            使用后台账号登录，权限由角色统一控制。
          </Typography.Text>
        </Space>
        {error && <Alert type="error" showIcon message={error} style={{ marginBottom: 16 }} />}
        <Form<LoginValues>
          layout="vertical"
          initialValues={{ username: 'admin' }}
          onFinish={(values) => void submit(values)}
          onValuesChange={() => { if (error) setError('') }}
        >
          <Form.Item label="用户名" name="username" rules={[{ required: true }]}>
            <Input autoComplete="username" />
          </Form.Item>
          <Form.Item label="密码" name="password" rules={[{ required: true }]}>
            <Input.Password autoComplete="current-password" />
          </Form.Item>
          <Button type="primary" htmlType="submit" block size="large" loading={submitting}>
            登录
          </Button>
        </Form>
      </Card>
    </main>
  )
}
