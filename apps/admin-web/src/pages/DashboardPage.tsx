import { Alert, Card, Col, Row, Skeleton, Statistic, Typography } from 'antd'
import { useQuery } from '@tanstack/react-query'
import { apiRequest } from '../api/http'
import { statusLabel } from '../utils/enums'

interface PingResponse {
  service: string
  status: string
  time: string
}

export function DashboardPage() {
  const ping = useQuery({
    queryKey: ['system', 'ping'],
    queryFn: () => apiRequest<PingResponse>('/api/v1/public/ping'),
  })

  return (
    <>
      <Typography.Title level={3}>经营概览</Typography.Title>
      {ping.isError && (
        <Alert
          type="warning"
          showIcon
          message="后端尚未连接"
          description="启动 Go 服务和本地 PostgreSQL 后，这里会显示实时健康状态。"
          style={{ marginBottom: 16 }}
        />
      )}
      <Row gutter={[16, 16]}>
        <Col span={6}><Card><Statistic title="今日订单" value={0} /></Card></Col>
        <Col span={6}><Card><Statistic title="待派工单" value={0} /></Card></Col>
        <Col span={6}><Card><Statistic title="待确认报价" value={0} /></Card></Col>
        <Col span={6}><Card><Statistic title="售后处理中" value={0} /></Card></Col>
      </Row>
      <Card title="系统连接" style={{ marginTop: 16 }}>
        {ping.isLoading ? (
          <Skeleton active paragraph={{ rows: 1 }} />
        ) : (
          <Typography.Text>
            {ping.data ? `${ping.data.service} · ${statusLabel(ping.data.status)} · ${ping.data.time}` : '等待后端服务'}
          </Typography.Text>
        )}
      </Card>
    </>
  )
}
