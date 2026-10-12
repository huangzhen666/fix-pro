import { apiRequest } from './http'
export type Usage = { orderCount: number; completedCount: number; cancelledCount: number; paidAmount: number; lastOrderAt?: string }
export type Customer = { id: string; displayName: string; status: string; mobileMasked: string; avatarUrl?: string; createdAt: string; lastLoginAt?: string; lastActivityAt?: string; usage: Usage }
export type CustomerPage = { items: Customer[]; total: number; page: number; pageSize: number }
export type Overview = { registered: number; newToday: number; new7Days: number; activeToday: number; active7Days: number; active30Days: number; asOf: string; timezone: string }
export type CustomerDetail = { customer: Customer; addressCount: number; cartItemCount: number }
export type CustomerOrders = { items: { id: string; orderNo: string; status: string; totalAmount: number; paidAmount: number; createdAt: string }[]; total: number }
const root = '/api/v1/admin/customers'
export const customerOverview = () => apiRequest<Overview>(root + '/overview')
export const listCustomers = (query: URLSearchParams) => apiRequest<CustomerPage>(root + '?' + query)
export const customerDetail = (id: string) => apiRequest<CustomerDetail>(root + '/' + id)
export const customerOrders = (id: string, page: number) => apiRequest<CustomerOrders>(root + '/' + id + '/orders?page=' + page + '&pageSize=10')
