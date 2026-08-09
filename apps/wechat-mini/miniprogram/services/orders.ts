import { request } from './request'
export interface OrderResult { id:string;orderNo:string;status:string;totalAmount:number;createdAt:string }
export const createOrder=(data:{contactName:string;contactMobile:string;serviceAddress:string},key:string)=>request<OrderResult>({url:'/api/v1/mini/orders',method:'POST',data,header:{'Idempotency-Key':key}})
