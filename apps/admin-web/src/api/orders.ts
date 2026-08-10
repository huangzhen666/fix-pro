import { apiRequest } from './http'
import type { PageResult } from './catalog'

export interface OrderSummary { id: string; orderNo: string; status: string; contactName: string; contactMobile: string; totalAmount: number; itemCount: number; createdAt: string; version: number; appointmentAt?: string; appointmentSlot?: string }
export interface FaultMedia { id: string; mediaType: 'IMAGE' | 'VIDEO'; name: string; url: string }
export interface OrderItem { id: string; skuCode: string; skuName: string; skuVersion: number; unit: string; serviceScope: string; exclusions: string; warrantyDescription: string; coverImageUrl: string; faultDescription: string; unitPrice: number; quantity: number; subtotal: number; faultMedia: FaultMedia[] }
export interface OrderDetail { order: OrderSummary & { customerId: string; contactMobile: string; serviceAddress: string; version: number; appointmentAt?: string; appointmentSlot?: string }; items: OrderItem[] }
export const listOrders = (keyword = '', status = '', contact = '', createdFrom = '', createdTo = '', page = 1, pageSize = 20) => apiRequest<PageResult<OrderSummary>>(`/api/v1/admin/orders?page=${page}&pageSize=${pageSize}&keyword=${encodeURIComponent(keyword)}&status=${encodeURIComponent(status)}&contact=${encodeURIComponent(contact)}&createdFrom=${createdFrom}&createdTo=${createdTo}`)
export const getOrder = (id: string) => apiRequest<OrderDetail>(`/api/v1/admin/orders/${id}`)
export const confirmAndCreateWorkOrders = (id: string, version: number) => apiRequest<{orderId: string; orderStatus: string; workOrders: {id: string; workOrderNo: string; status: string}[]}>(`/api/v1/admin/orders/${id}/confirm`, { method: 'POST', headers: { 'Idempotency-Key': crypto.randomUUID() }, body: JSON.stringify({ version, priority: 'NORMAL' }) })
