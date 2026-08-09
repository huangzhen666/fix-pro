import { useState } from 'react'
import { Card, Input, Space, Table, Tag, Typography } from 'antd'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router'
import { listOrders } from '../api/orders'

export default function OrderListPage(){const [keyword,setKeyword]=useState('');const navigate=useNavigate();const query=useQuery({queryKey:['orders',keyword],queryFn:()=>listOrders(keyword)});return <Card><Space direction="vertical" size="large" style={{width:'100%'}}><Typography.Title level={3} style={{margin:0}}>订单中心</Typography.Title><Input.Search allowClear placeholder="输入订单号" style={{width:360}} onSearch={setKeyword}/><Table rowKey="id" loading={query.isLoading} dataSource={query.data?.items} pagination={false} onRow={r=>({onClick:()=>navigate(`/orders/${r.id}`),style:{cursor:'pointer'}})} columns={[{title:'订单号',dataIndex:'orderNo'},{title:'状态',dataIndex:'status',render:v=><Tag color="gold">{v}</Tag>},{title:'联系人',dataIndex:'contactName'},{title:'手机号',dataIndex:'contactMobile'},{title:'项目数',dataIndex:'itemCount'},{title:'总金额',dataIndex:'totalAmount',render:v=>`¥${(v/100).toFixed(2)}`},{title:'创建时间',dataIndex:'createdAt',render:v=>new Date(v).toLocaleString()}]}/></Space></Card>}
