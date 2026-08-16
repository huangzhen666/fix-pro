import { apiRequest, uploadFile } from './http'

export interface Trade { id: string; tradeCode: string; name: string; description?: string; sortOrder: number; status: string; version: number; skillCount: number }
export interface Skill { id: string; tradeId: string; tradeName?: string; skillCode: string; name: string; description?: string; sortOrder: number; status: string; version: number }
export interface WorkerMedia { id: string; mediaType: 'IMAGE' | 'VIDEO'; contentType: string; name: string; url: string; createdAt: string }
export interface Worker { id: string; workerNo: string; displayName: string; mobileMasked?: string; mobile?: string; status: string; mustChangePassword?: boolean; version: number; openWorkOrderCount: number; initialPassword?: string; trades?: string[]; skills?: string[]; tradeIds?: number[]; skillIds?: number[]; avatar?: WorkerMedia; certificates?: WorkerMedia[] }
export interface Candidate extends Worker { matchedSkills: string[]; matchedSkillCount: number; requiredSkillCount: number; allSkillsMatched: boolean }
export interface WorkerWrite { displayName: string; mobile: string; tradeIds: number[]; skillIds: number[]; joinedOn?: string; remark?: string; activate?: boolean; version: number; avatarMediaId?: number; certificateMediaIds?: number[] }
export const uploadWorkerMedia = (purpose: 'AVATAR' | 'CERTIFICATE', file: File) => uploadFile(`/api/v1/admin/media/worker?purpose=${purpose}`, file)
export const listTrades = (status = '') => apiRequest<Trade[]>(`/api/v1/admin/worker-trades?status=${encodeURIComponent(status)}`)
export const createTrade = (body: Partial<Trade>) => apiRequest<Trade>('/api/v1/admin/worker-trades', { method: 'POST', body: JSON.stringify(body) })
export const updateTrade = (id: string, body: Partial<Trade>) => apiRequest<Trade>(`/api/v1/admin/worker-trades/${id}`, { method: 'PUT', body: JSON.stringify(body) })
export const setTradeStatus = (id: string, status: string, version: number) => apiRequest<Trade>(`/api/v1/admin/worker-trades/${id}/status`, { method: 'POST', body: JSON.stringify({ status, version }) })
export const deleteTrade = (id: string) => apiRequest<{ deleted: boolean }>(`/api/v1/admin/worker-trades/${id}`, { method: 'DELETE' })
export const listSkills = (tradeId = '', status = '') => apiRequest<Skill[]>(`/api/v1/admin/worker-skills?tradeId=${tradeId}&status=${status}`)
export const createSkill = (body: Partial<Skill>) => apiRequest<Skill>('/api/v1/admin/worker-skills', { method: 'POST', body: JSON.stringify(body) })
export const updateSkill = (id: string, body: Partial<Skill>) => apiRequest<Skill>(`/api/v1/admin/worker-skills/${id}`, { method: 'PUT', body: JSON.stringify(body) })
export const setSkillStatus = (id: string, status: string, version: number) => apiRequest<Skill>(`/api/v1/admin/worker-skills/${id}/status`, { method: 'POST', body: JSON.stringify({ status, version }) })
export const deleteSkill = (id: string) => apiRequest<{ deleted: boolean }>(`/api/v1/admin/worker-skills/${id}`, { method: 'DELETE' })
export const listWorkers = (status = '', keyword = '') => apiRequest<Worker[]>(`/api/v1/admin/workers?status=${status}&keyword=${encodeURIComponent(keyword)}`)
export const getWorker = (id: string) => apiRequest<Worker>(`/api/v1/admin/workers/${id}`)
export const saveWorker = (id: string | undefined, body: WorkerWrite) => apiRequest<Worker>(id ? `/api/v1/admin/workers/${id}` : '/api/v1/admin/workers', { method: id ? 'PUT' : 'POST', body: JSON.stringify(body) })
export const activateWorker = (id: string, version: number) => apiRequest(`/api/v1/admin/workers/${id}/activate`, { method: 'POST', body: JSON.stringify({ version }) })
export const disableWorker = (id: string, body: { reason: string; workOrderPolicy: string; version: number }) => apiRequest(`/api/v1/admin/workers/${id}/disable`, { method: 'POST', body: JSON.stringify(body) })
export const resetWorkerPassword = (id: string) => apiRequest<{ workerId: number; temporaryPassword: string; mustChangePassword: boolean }>(`/api/v1/admin/workers/${id}/reset-password`, { method: 'POST', body: JSON.stringify({ confirm: true }) })
export const listCandidates = (workOrderId: string) => apiRequest<Candidate[]>(`/api/v1/admin/workers/candidates?workOrderId=${workOrderId}`)
