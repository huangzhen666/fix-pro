import { request } from './request'
export interface OrderResult { id:string;orderNo:string;status:string;totalAmount:number;createdAt:string }
export const createOrder=(data:{contactName:string;contactMobile:string;serviceAddress:string;appointmentDate:string;appointmentSlot:string},key:string)=>request<OrderResult>({url:'/api/v1/mini/orders',method:'POST',data,header:{'Idempotency-Key':key}})
export interface CustomerOrderSummary { id:string; orderNo:string; status:string; statusText?:string; totalAmount:number; itemCount:number; workOrderTotal:number; workOrderFinished:number; createdAt:string }
export interface CustomerEvidence { id:string; mediaId:string; stage:string; url:string; createdAt:string }
export interface CustomerTimelineEvent { code:string; operatorType:string; note?:string; createdAt:string }
export interface CustomerWorkOrder { id:string; workOrderNo:string; status:string; statusText?:string; assigneeName?:string; appointmentAt?:string; appointmentSlot?:string; appointmentSlotLabel?:string; completionSummary?:string; version:number; evidence:CustomerEvidence[] }
export interface CustomerOrderDetail { id:string; orderNo:string; status:string; statusText?:string; contactName:string; contactMobile:string; serviceAddress:string; totalAmount:number; version:number; createdAt:string; appointmentAt?:string; appointmentSlot?:string; appointmentSlotLabel?:string; workOrders:CustomerWorkOrder[] }
export const listCustomerOrders=()=>request<{items:CustomerOrderSummary[];total:number}>({url:'/api/v1/mini/orders?page=1&pageSize=50',method:'GET'})
export const getCustomerOrder=(id:string)=>request<CustomerOrderDetail>({url:`/api/v1/mini/orders/${id}`,method:'GET'})
export const getWorkOrderTimeline=(id:string)=>request<{items:CustomerTimelineEvent[]}>({url:`/api/v1/mini/work-orders/${id}/timeline`,method:'GET'})
export const acceptWorkOrder=(id:string,decision:'ACCEPT'|'REJECT',version:number,reason='')=>request<{updated:boolean}>({url:`/api/v1/mini/work-orders/${id}/acceptance`,method:'POST',data:{decision,version,reason},header:{'Idempotency-Key':`${id}-${version}-${decision}`}})
export const rateWorkOrder=(id:string,stars:number,content:string,version:number)=>request<{created:boolean}>({url:`/api/v1/mini/work-orders/${id}/rating`,method:'POST',data:{stars,content,version},header:{'Idempotency-Key':`${id}-${version}-rating`}})
