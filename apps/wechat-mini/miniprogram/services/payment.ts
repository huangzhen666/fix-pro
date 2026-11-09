import { request } from './request'
import { isDevtools, getApiBaseUrl } from '../config/env'
export interface PaymentRecord { id:string; channel:'MOCK'|'WECHAT'; status:string; amount:number; outTradeNo:string; expiresAt:string }
export interface PaymentOverview { required:boolean; status:string; totalAmount:number; paidAmount:number; refundedAmount:number; refundReserved:number; expiresAt?:string; payments:PaymentRecord[]; mockEnabled:boolean; wechatEnabled:boolean }
interface Prepared { payment:PaymentRecord; ticket?:string; cloudEnv?:string; cloudFunction?:string }
export const getPayment = (id:string) => request<PaymentOverview>({url:`/api/v1/mini/orders/${id}/payment`,method:'GET'})
export async function payOrder(id:string):Promise<void> {
  const tools=isDevtools()
  const overview=await getPayment(id)
  const local=/^http:\/\/(localhost|127\.0\.0\.1)(:\d+)?$/.test(getApiBaseUrl())
  if (tools && (!local || !overview.mockEnabled)) throw new Error('开发者工具仅支持已启用的本地模拟支付；真实支付请使用真机')
  if (!tools && !overview.wechatEnabled) throw new Error('真机支付需连接已配置微信支付的联调或正式服务')
  const channel=tools?'MOCK':'WECHAT'
  const prepared=await request<Prepared>({url:`/api/v1/mini/orders/${id}/payment`,method:'POST',data:{channel}})
  if (prepared.payment.channel!==channel) throw new Error('支付通道不匹配')
  if (channel==='MOCK') {
    const choice=await new Promise<WechatMiniprogram.ShowActionSheetSuccessCallbackResult>((resolve,reject)=>wx.showActionSheet({itemList:['模拟支付成功（不扣款）','模拟取消','模拟失败','模拟处理中'],success:resolve,fail:reject}))
    const outcome=['SUCCESS','CANCEL','FAIL','PENDING'][choice.tapIndex]
    await request({url:`/api/v1/mini/orders/${id}/payment/mock`,method:'POST',data:{outcome}})
    if(outcome!=='SUCCESS') throw new Error(({CANCEL:'已取消模拟付款，订单仍待支付',FAIL:'模拟付款失败，订单仍待支付',PENDING:'模拟处理中，可再次发起模拟确认'} as Record<string,string>)[outcome])
    return
  }
  if (!prepared.ticket || !prepared.cloudEnv || !prepared.cloudFunction) throw new Error('支付配置缺失')
  wx.cloud.init({env:prepared.cloudEnv,traceUser:true})
  // This bridge preserves the tutorial's double data response contract.
  const cloud=wx.cloud as typeof wx.cloud & {callHTTPFunction:(options:{name:string;config:{env:string};path:string;method:string;header:Record<string,string>;data:{ticket:string}})=>Promise<{data:unknown}>}
  if(typeof cloud.callHTTPFunction!=='function') throw new Error('微信基础库不支持支付云函数，请升级微信')
  const result=await cloud.callHTTPFunction({name:prepared.cloudFunction,config:{env:prepared.cloudEnv},path:'/wx-pay/prepay',method:'POST',header:{'Content-Type':'application/json'},data:{ticket:prepared.ticket}})
  const body=(typeof result.data==='string'?JSON.parse(result.data):result.data) as {code:number;data?:{data?:WechatMiniprogram.RequestPaymentOption};message?:string}
  if (body.code!==0 || !body.data?.data) throw new Error(body.message||'预支付未完成，请查询订单结果')
  const p=body.data.data
  await new Promise<void>((resolve,reject)=>wx.requestPayment({timeStamp:String(p.timeStamp),nonceStr:p.nonceStr,package:p.package,signType:p.signType,paySign:p.paySign,success:()=>resolve(),fail:e=>reject(new Error(e.errMsg.includes('cancel')?'已取消付款，订单仍待支付':'付款结果确认中，请查询订单'))}))
}
export async function waitPayment(id:string):Promise<PaymentOverview> {
  let result=await getPayment(id)
  for(let i=0;i<4 && result.paidAmount===0;i++) { await new Promise<void>(resolve=>setTimeout(resolve,800*(i+1)));result=await getPayment(id) }
  return result
}
