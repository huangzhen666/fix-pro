import { apiRequest } from './http'

export interface PageResult<T> { items: T[]; total: number; page: number; pageSize: number }
export interface Category { id: string; parentId?: string; name: string; sortOrder: number; status: 'ACTIVE'|'DISABLED'; skuCount: number }
export interface SkuSummary { id: string; skuCode: string; name: string; categoryName: string; basePrice: number; unit: string; status: string; publishedVersion?: number; createdAt: string; updatedAt: string; createdBy: string; updatedBy: string }
export interface SkuDetail { id: string; categoryId: string; skuCode: string; name: string; description?: string; serviceScope: string; exclusions: string; warrantyDescription: string; priceMode: string; basePrice: number; unit: string; coverMediaId?: string; galleryMediaIds: string[]; requiredSkillIds: string[]; status: string; version: number; publishedVersion?: number; createdAt: string; updatedAt: string; createdBy: string; updatedBy: string }
export interface SkuWrite { categoryId: string; skuCode: string; name: string; description?: string; serviceScope: string; exclusions: string; warrantyDescription: string; priceMode: string; basePrice: number; unit: string; coverMediaId: string; galleryMediaIds: string[]; requiredSkillIds: string[]; version: number }

export const listCategories = () => apiRequest<Category[]>('/api/v1/admin/catalog/categories')
export const listAllCategories = () => apiRequest<Category[]>('/api/v1/admin/catalog/categories?includeDisabled=true')
export const createCategory = (body: { parentId?: string; name: string; sortOrder: number }) => apiRequest<Category>('/api/v1/admin/catalog/categories', { method:'POST', body:JSON.stringify(body) })
export const updateCategory = (id:string, body: { parentId?: string; name: string; sortOrder: number }) => apiRequest<Category>(`/api/v1/admin/catalog/categories/${id}`, { method:'PUT', body:JSON.stringify(body) })
export const setCategoryStatus = (id:string,status:'ACTIVE'|'DISABLED') => apiRequest<Category>(`/api/v1/admin/catalog/categories/${id}/status`, { method:'POST', body:JSON.stringify({status}) })
export const listSkus = (keyword = '', categoryId = '', page = 1) => apiRequest<PageResult<SkuSummary>>(`/api/v1/admin/catalog/skus?page=${page}&pageSize=20&keyword=${encodeURIComponent(keyword)}&categoryId=${encodeURIComponent(categoryId)}`)
export const getSku = (id: string) => apiRequest<SkuDetail>(`/api/v1/admin/catalog/skus/${id}`)
export const createSku = (body: SkuWrite) => apiRequest<SkuDetail>('/api/v1/admin/catalog/skus', { method: 'POST', body: JSON.stringify(body) })
export const updateSku = (id: string, body: SkuWrite) => apiRequest<SkuDetail>(`/api/v1/admin/catalog/skus/${id}`, { method: 'PUT', body: JSON.stringify(body) })
export const publishSku = (id: string) => apiRequest(`/api/v1/admin/catalog/skus/${id}/publish`, { method: 'POST' })
export const offShelfSku = (id: string) => apiRequest(`/api/v1/admin/catalog/skus/${id}/off-shelf`, { method: 'POST' })
