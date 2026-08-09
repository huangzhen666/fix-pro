import { useState } from 'react'
import { App, Button, Card, Input, Space, Table, Tag, Typography } from 'antd'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from 'react-router'
import { listSkus, offShelfSku, publishSku } from '../api/catalog'

const money = (value: number) => `¥${(value / 100).toFixed(2)}`
export default function SkuListPage() {
  const [keyword, setKeyword] = useState(''); const navigate = useNavigate(); const client = useQueryClient(); const { message } = App.useApp()
  const query = useQuery({ queryKey: ['skus', keyword], queryFn: () => listSkus(keyword) })
  const action = useMutation({ mutationFn: ({ id, type }: { id: string; type: 'publish' | 'off' }) => type === 'publish' ? publishSku(id) : offShelfSku(id), onSuccess: () => { message.success('操作成功'); client.invalidateQueries({ queryKey: ['skus'] }) }, onError: (e) => message.error(e.message) })
  return <Card><Space direction="vertical" size="large" style={{ width: '100%' }}>
    <Space style={{ justifyContent: 'space-between', width: '100%' }}><Typography.Title level={3} style={{ margin: 0 }}>维修 SKU</Typography.Title><Button type="primary" onClick={() => navigate('/catalog/skus/new')}>新增 SKU</Button></Space>
    <Input.Search allowClear placeholder="名称或 SKU 编码" style={{ width: 360 }} onSearch={setKeyword} />
    <Table rowKey="id" loading={query.isLoading} dataSource={query.data?.items} pagination={false} columns={[
      { title: '编码', dataIndex: 'skuCode' }, { title: '名称', dataIndex: 'name' }, { title: '分类', dataIndex: 'categoryName' },
      { title: '价格', render: (_, r) => `${money(r.basePrice)}/${r.unit}` }, { title: '状态', dataIndex: 'status', render: (v) => <Tag color={v === 'PUBLISHED' ? 'green' : v === 'DRAFT' ? 'blue' : 'default'}>{v}</Tag> },
      { title: '版本', dataIndex: 'publishedVersion', render: (v) => v ?? '-' },
      { title: '操作', render: (_, r) => <Space><Button size="small" onClick={() => navigate(`/catalog/skus/${r.id}/edit`)}>编辑</Button><Button size="small" type="primary" loading={action.isPending} onClick={() => action.mutate({ id: r.id, type: 'publish' })}>发布</Button>{r.status === 'PUBLISHED' && <Button size="small" danger onClick={() => action.mutate({ id: r.id, type: 'off' })}>下架</Button>}</Space> },
    ]} />
  </Space></Card>
}
