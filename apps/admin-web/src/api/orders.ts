import { apiRequest } from './http'
import type { PageResult } from './catalog'

export interface OrderSummary { id: string; orderNo: string; status: string; contactName: string; contactMobile: string; totalAmount: number; itemCount: number; createdAt: string }
export interface FaultMedia { id: string; mediaType: 'IMAGE' | 'VIDEO'; name: string; url: string }
export interface OrderItem { id: string; skuCode: string; skuName: string; skuVersion: number; unit: string; serviceScope: string; exclusions: string; warrantyDescription: string; coverImageUrl: string; faultDescription: string; unitPrice: number; quantity: number; subtotal: number; faultMedia: FaultMedia[] }
export interface OrderDetail { order: OrderSummary & { customerId: string; contactMobile: string; serviceAddress: string }; items: OrderItem[] }
export const listOrders = (keyword = '', page = 1) => apiRequest<PageResult<OrderSummary>>(`/api/v1/admin/orders?page=${page}&pageSize=20&keyword=${encodeURIComponent(keyword)}`)
export const getOrder = (id: string) => apiRequest<OrderDetail>(`/api/v1/admin/orders/${id}`)
