import { request } from './request'

export interface ServiceSummary {
  id: string
  skuCode: string
  name: string
  description?: string
  serviceScope: string
  exclusions: string
  warrantyDescription: string
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

export function getService(id: string): Promise<ServiceSummary> {
  return request<ServiceSummary>({ url: `/api/v1/catalog/services/${id}`, method: 'GET' })
}
