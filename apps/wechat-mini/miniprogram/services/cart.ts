import type { ServiceProfile } from './serviceProfile'
import { request, upload } from './request'
import { hasSession } from './session'
export interface DemandMedia { id: string; mediaType: 'IMAGE'|'VIDEO'; name: string; url: string }
export interface CartItem { detailProfile:ServiceProfile; id:string;skuId:string;skuVersion:number;name:string;coverImageUrl:string;unitPrice:number;unit:string;quantity:number;subtotal:number;demandDescription?:string;demandMedia:DemandMedia[];demandComplete:boolean;available:boolean;needsConfirmation:boolean;currentVersion:number }
export interface Cart { items:CartItem[];itemCount:number;totalAmount:number }
export const getCart=()=>hasSession()?request<Cart>({url:'/api/v1/mini/cart',method:'GET'}):Promise.resolve<Cart>({items:[],itemCount:0,totalAmount:0})
export const addCart=(skuId:string,quantity:number)=>request<Cart>({url:'/api/v1/mini/cart/items',method:'POST',data:{skuId,quantity}})
export const updateQuantity=(id:string,quantity:number)=>request<Cart>({url:`/api/v1/mini/cart/items/${id}`,method:'PATCH' as any,data:{quantity}})
export const removeItem=(id:string)=>request<Cart>({url:`/api/v1/mini/cart/items/${id}`,method:'DELETE'})
export const saveDemand=(id:string,demandDescription:string,mediaIds:string[])=>request<Cart>({url:`/api/v1/mini/cart/items/${id}/demand`,method:'PUT',data:{demandDescription,mediaIds}})
export const uploadDemand=(filePath:string)=>upload<{id:number;mediaType:'IMAGE'|'VIDEO'}>('/api/v1/mini/media/demand',filePath)

export const confirmCartItem=(id:string,skuVersion:number)=>request<Cart>({url:`/api/v1/mini/cart/items/${id}/confirm`,method:"POST",data:{skuVersion}})
