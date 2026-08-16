import { request, upload } from './request'

export interface WorkOrder {
  id: string
  workOrderNo: string
  orderNo: string
  status: string
  statusText?: string
  appointmentAt?: string
  appointmentSlot?: string
  appointmentSlotLabel?: string
  appointmentDateText?: string
  customerName?: string
  customerMobile?: string
  serviceAddress?: string
  items?: WorkOrderItem[]
  completionSummary?: string
  version: number
  evidence?: Evidence[]
}

export interface WorkOrderItem {
  id: string
  name: string
  unit: string
  quantity: number
  customerNote?: string
  customerMedia: WorkOrderMedia[]
}

export interface WorkOrderMedia {
  id: string
  mediaType: 'IMAGE' | 'VIDEO' | string
  url: string
  localUrl?: string
}

export interface Evidence {
  id: string
  mediaId: string
  stage: 'BEFORE' | 'DURING' | 'AFTER'
  url: string
  createdAt: string
}

export interface WorkOrderPage {
  items: WorkOrder[]
  total: number
}

export function listWorkOrders(): Promise<WorkOrderPage> {
  return request<WorkOrderPage>({ url: '/api/v1/worker/work-orders' })
}

export function getWorkOrder(id: string): Promise<WorkOrder> {
  return request<WorkOrder>({ url: `/api/v1/worker/work-orders/${id}` })
}

export function commandWorkOrder(id: string, command: 'ACCEPT' | 'REJECT' | 'ARRIVE' | 'START', version: number, reason = ''): Promise<{ updated: boolean }> {
  return request({ url: `/api/v1/worker/work-orders/${id}/${command}`, method: 'POST', data: { version, reason }, header: { 'Idempotency-Key': `${id}-${version}-${command}` } })
}

export function uploadEvidence(id: string, filePath: string): Promise<{ id: string; mediaType: string }> {
  return upload(`/api/v1/worker/work-orders/${id}/media/images`, filePath)
}

export function bindEvidence(id: string, mediaId: string, stage: Evidence['stage'], version: number): Promise<{ updated: boolean }> {
  return request({ url: `/api/v1/worker/work-orders/${id}/evidence`, method: 'POST', data: { mediaId: Number(mediaId), stage, customerVisible: true, version } })
}

export function submitCompletion(id: string, completionSummary: string, version: number): Promise<{ updated: boolean }> {
  return request({ url: `/api/v1/worker/work-orders/${id}/submit-completion`, method: 'POST', data: { completionSummary, version }, header: { 'Idempotency-Key': `${id}-${version}-SUBMIT_COMPLETION` } })
}
export function workerReschedule(id: string, appointmentAt: string, appointmentSlot: string, version: number): Promise<{ updated: boolean }> { return request({ url: `/api/v1/worker/work-orders/${id}/reschedule`, method: 'POST', data: { appointmentAt, appointmentSlot, version, communicationConfirmed: true } }) }
export function workerReturn(id: string, version: number, reason: string): Promise<{ updated: boolean }> { return request({ url: `/api/v1/worker/work-orders/${id}/return`, method: 'POST', data: { version, reason } }) }
