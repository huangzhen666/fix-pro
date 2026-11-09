import { request } from './request'
import type { DemandMedia } from './cart'
export type DemandDraftItem = { itemId: string; name: string; demandDescription: string; demandMedia: Array<DemandMedia & { localUrl?: string }> }
export type OrderDemand = { orderId: string; demandVersion: number; editable: boolean; readOnlyReason: string; items: DemandDraftItem[] }
export const getOrderDemand = (id: string) => request<OrderDemand>({ url: `/api/v1/mini/orders/${id}/demand` })
export const updateOrderDemand = (id: string, demandVersion: number, items: DemandDraftItem[], key: string) => request<OrderDemand>({
  url: `/api/v1/mini/orders/${id}/demand`, method: 'PUT', header: { 'Idempotency-Key': key },
  data: { demandVersion, items: items.map(item => ({ itemId: item.itemId, demandDescription: item.demandDescription, mediaIds: item.demandMedia.map(media => media.id) })) },
})
export const demandFilled = (items: DemandDraftItem[]) => items.some(item => item.demandDescription.trim() || item.demandMedia.length)
