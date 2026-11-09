import { useEffect, useRef, useState } from 'react'
import { Alert, Button, Descriptions, Input, Modal, Space, Table, Tag, message } from 'antd'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiRequest } from '../api/http'
import { useAuthStore } from '../stores/authStore'
interface Payment { lastError?:string; id:string;channel:string;status:string;amount:number;transactionId:string;outTradeNo:string }
interface Refund { id:string;outRefundNo:string;status:string;amount:number;reason:string }
interface Overview { required:boolean;status:string;totalAmount:number;paidAmount:number;refundedAmount:number;refundReserved:number;payments:Payment[];refunds:Refund[] }
const money=(value:number)=>`¥${(value/100).toFixed(2)}`
const labels:Record<string,string>={UNPAID:'待支付',PAID:'已支付',EXEMPT:'历史订单免付',PARTIALLY_REFUNDED:'部分退款',REFUNDED:'已退款',REFUNDING:'退款处理中',SUCCESS:'成功',PROCESSING:'处理中',ABNORMAL:'异常待处理',CLOSED:'已关闭',PENDING:'待确认'}
export function OrderPayment({orderId}:{orderId:string}) {
 const client=useQueryClient(), auth=useAuthStore()
 const [open,setOpen]=useState(false),[amount,setAmount]=useState(''),[reason,setReason]=useState('')
 const key=useRef('')
 const query=useQuery({queryKey:['order-payment',orderId],queryFn:()=>apiRequest<Overview>(`/api/v1/admin/orders/${orderId}/payment`),refetchInterval:5000})
 const paid=query.data?.paidAmount, refunded=query.data?.refundedAmount, reserved=query.data?.refundReserved
 useEffect(()=>{if(paid!==undefined){client.invalidateQueries({queryKey:['order-drawer',orderId]});client.invalidateQueries({queryKey:['orders']})}},[paid,refunded,reserved,client,orderId])
 const refund=useMutation({mutationFn:()=>apiRequest(`/api/v1/admin/orders/${orderId}/refunds`,{method:'POST',headers:{'Idempotency-Key':key.current},body:JSON.stringify({amount:Math.round(Number(amount)*100),reason:reason.trim()})}),onSuccess:()=>{message.success('退款申请已受理，请以最终退款状态为准');setOpen(false);client.invalidateQueries({queryKey:['order-payment',orderId]});client.invalidateQueries({queryKey:['order-drawer',orderId]});client.invalidateQueries({queryKey:['orders']})},onError:(e:Error)=>message.error(e.message)})
 if(query.isError)return <Alert type="error" message="支付记录加载失败" action={<Button onClick={()=>query.refetch()}>重试</Button>}/>
 if(!query.data)return null
 const data=query.data, available=data.paidAmount-data.refundedAmount-data.refundReserved
 const canRefund=auth.user?.platformSuperAdmin||auth.permissions.includes('payment.refund')
 return <Space direction="vertical" style={{width:'100%'}}>
  {data.payments.some(p=>p.lastError==='LATE_PAYMENT_REQUIRES_REFUND')&&<Alert type="error" message="已取消订单收到付款，请核对并退款，勿恢复履约"/>}
  {data.payments.some(p=>p.lastError && p.lastError!=='LATE_PAYMENT_REQUIRES_REFUND')&&<Alert type="warning" message="渠道核对异常，系统将重试，请关注交易状态"/>}
  {data.payments.some(p=>p.channel==='MOCK')&&<Alert type="warning" message="模拟支付记录：未发生真实扣款或退款，不计入真实交易汇总"/>}
  <Descriptions size="small" bordered items={[{key:'status',label:'付款状态',children:labels[data.status]||data.status},{key:'paid',label:'实付',children:money(data.paidAmount)},{key:'refund',label:'已退款',children:money(data.refundedAmount)},{key:'reserved',label:'退款处理中/异常预留',children:money(data.refundReserved)}]}/>
  <Table size="small" rowKey="id" pagination={false} dataSource={data.payments} columns={[{title:'支付单号',dataIndex:'outTradeNo'},{title:'通道',dataIndex:'channel',render:v=><Tag color={v==='MOCK'?'orange':'green'}>{v==='MOCK'?'模拟支付':'微信支付'}</Tag>},{title:'状态',dataIndex:'status',render:v=>labels[v]||v},{title:'金额',dataIndex:'amount',render:money},{title:'渠道流水',dataIndex:'transactionId'}]}/>
  <Table size="small" rowKey="id" pagination={false} dataSource={data.refunds} columns={[{title:'退款单号',dataIndex:'outRefundNo'},{title:'状态',dataIndex:'status',render:v=>labels[v]||v},{title:'金额',dataIndex:'amount',render:money},{title:'原因',dataIndex:'reason'}]}/>
  {canRefund&&available>0&&<Button danger onClick={()=>{key.current=crypto.randomUUID();setAmount((available/100).toFixed(2));setReason('');setOpen(true)}}>申请退款</Button>}
  <Modal title="申请退款" open={open} confirmLoading={refund.isPending} onCancel={()=>{if(!refund.isPending)setOpen(false)}} onOk={()=>{if(!/^\d+(\.\d{1,2})?$/.test(amount)||Number(amount)<=0||Math.round(Number(amount)*100)>available||!reason.trim()){message.warning('请填写有效金额及退款原因');return};refund.mutate()}}>
   <p>可退金额 {money(available)}，退款受理后需等待渠道确认。</p>
   <Input disabled={refund.isPending} addonBefore="退款金额（元）" value={amount} onChange={e=>{setAmount(e.target.value);key.current=crypto.randomUUID()}}/>
   <Input.TextArea disabled={refund.isPending} style={{marginTop:12}} placeholder="退款原因（必填）" maxLength={512} value={reason} onChange={e=>{setReason(e.target.value);key.current=crypto.randomUUID()}}/>
  </Modal>
 </Space>
}
