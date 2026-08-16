import { App, Button, Card, Form, Input, Space, Typography } from 'antd'
import { useNavigate } from 'react-router'
import { apiRequest } from '../api/http'
import { useAuthStore } from '../stores/authStore'

export default function ChangePasswordPage() {
  const { message } = App.useApp()
  const navigate = useNavigate()
  const clearSession = useAuthStore((state) => state.clearSession)
  async function submit(values: { currentPassword: string; newPassword: string; confirmPassword: string }) {
    try {
      await apiRequest('/api/v1/admin/auth/password', { method: 'POST', body: JSON.stringify(values) })
      try { await apiRequest('/api/v1/admin/auth/logout', { method: 'POST' }) } catch { /* session may already be expired */ }
      clearSession()
      message.success('密码已修改，请使用新密码重新登录')
      navigate('/login', { replace: true })
    } catch (e) { if (e instanceof Error) message.error(e.message) }
  }
  return <main className="login-shell"><Card className="login-card" bordered={false}>
    <Space direction="vertical" size={4} style={{ marginBottom: 24 }}><Typography.Title level={2} style={{ margin: 0 }}>修改初始密码</Typography.Title><Typography.Text type="secondary">首次登录必须设置新的后台密码。</Typography.Text></Space>
    <Form layout="vertical" onFinish={(values) => void submit(values)}>
      <Form.Item name="currentPassword" label="当前密码" rules={[{ required: true }]}><Input.Password autoComplete="current-password" /></Form.Item>
      <Form.Item name="newPassword" label="新密码" rules={[{ required: true, min: 12, message: '密码至少 12 位' }]}><Input.Password autoComplete="new-password" /></Form.Item>
      <Form.Item name="confirmPassword" label="确认新密码" dependencies={['newPassword']} rules={[{ required: true, message: '请再次输入新密码' }, ({ getFieldValue }) => ({ validator(_, value) { return !value || getFieldValue('newPassword') === value ? Promise.resolve() : Promise.reject(new Error('两次输入的新密码不一致')) } })]}><Input.Password autoComplete="new-password" /></Form.Item>
      <Button type="primary" htmlType="submit" block size="large">保存新密码</Button>
    </Form>
  </Card></main>
}
