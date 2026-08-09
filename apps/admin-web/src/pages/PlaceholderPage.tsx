import { Empty, Typography } from 'antd'

export function PlaceholderPage({ title }: { title: string }) {
  return (
    <>
      <Typography.Title level={3}>{title}</Typography.Title>
      <Empty description="模块边界已建立，等待对应 Sprint 实现" />
    </>
  )
}
