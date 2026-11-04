import { request } from './request'

export interface ServiceSummary {
  id: string
  skuCode: string
  name: string
  description?: string
  priceMode: 'FIXED'
  price: number
  unit: string
  coverImageUrl: string
  galleryImageUrls: string[]
  publishedVersion: number
}

export function listServices(): Promise<ServiceSummary[]> {
  return request<ServiceSummary[]>({ url: '/api/v1/catalog/services', method: 'GET' })
}

export function searchServices(keyword: string): Promise<ServiceSummary[]> {
  return request<ServiceSummary[]>({ url: `/api/v1/catalog/services?keyword=${encodeURIComponent(keyword)}`, method: 'GET' })
}

export interface CategoryGroup { id: string; name: string; services: ServiceSummary[] }
export function listCategoryGroups(): Promise<CategoryGroup[]> {
  return request<CategoryGroup[]>({ url: '/api/v1/catalog/categories', method: 'GET' })
}

export interface ServiceDetail extends Omit<ServiceSummary, "description"> { serviceDescription:string;serviceNotice:string }
export function getService(id: string): Promise<ServiceDetail> {
  return request<ServiceDetail>({ url: `/api/v1/catalog/services/${id}`, method: 'GET' })
}
