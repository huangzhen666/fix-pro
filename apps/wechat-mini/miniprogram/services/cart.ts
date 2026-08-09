import { request, upload } from './request'
export interface FaultMedia { id: string; mediaType: 'IMAGE'|'VIDEO'; name: string; url: string }
export interface CartItem { id:string;skuId:string;skuVersion:number;name:string;coverImageUrl:string;unitPrice:number;unit:string;quantity:number;subtotal:number;faultDescription?:string;faultMedia:FaultMedia[];faultComplete:boolean }
export interface Cart { items:CartItem[];itemCount:number;totalAmount:number }
export const getCart=()=>request<Cart>({url:'/api/v1/mini/cart',method:'GET'})
export const addCart=(skuId:string,quantity:number)=>request<Cart>({url:'/api/v1/mini/cart/items',method:'POST',data:{skuId,quantity}})
export const updateQuantity=(id:string,quantity:number)=>request<Cart>({url:`/api/v1/mini/cart/items/${id}`,method:'PATCH' as any,data:{quantity}})
export const removeItem=(id:string)=>request<Cart>({url:`/api/v1/mini/cart/items/${id}`,method:'DELETE'})
export const saveFault=(id:string,faultDescription:string,mediaIds:string[])=>request<Cart>({url:`/api/v1/mini/cart/items/${id}/fault`,method:'PUT',data:{faultDescription,mediaIds}})
export const uploadFault=(filePath:string)=>upload<{id:number;mediaType:'IMAGE'|'VIDEO'}>('/api/v1/mini/media/fault',filePath)
