import { useAuthStore } from '../stores/authStore'

export interface ApiResponse<T> {
  code: string
  message: string
  data: T
  requestId: string
}

export class ApiError extends Error {
  readonly code: string
  readonly requestId?: string

  constructor(message: string, code: string, requestId?: string) {
    super(message)
    this.code = code
    this.requestId = requestId
  }
}

const baseUrl = import.meta.env.VITE_API_BASE_URL ?? ''

export async function apiRequest<T>(path: string, init: RequestInit = {}): Promise<T> {
  const credential = useAuthStore.getState().credential
  const headers = new Headers(init.headers)
  headers.set('Accept', 'application/json')
  headers.set('X-Request-Id', crypto.randomUUID())
  if (init.body && !(init.body instanceof FormData)) headers.set('Content-Type', 'application/json')
  if (credential) headers.set('Authorization', `Basic ${credential}`)

  const response = await fetch(`${baseUrl}${path}`, { ...init, headers })
  const payload = (await response.json()) as ApiResponse<T>
  if (!response.ok || payload.code !== 'OK') {
    throw new ApiError(payload.message || '请求失败', payload.code, payload.requestId)
  }
  return payload.data
}

export async function apiBlob(path: string): Promise<Blob> {
  const credential = useAuthStore.getState().credential
  const headers = new Headers({ Accept: '*/*', 'X-Request-Id': crypto.randomUUID() })
  if (credential) headers.set('Authorization', `Basic ${credential}`)
  const response = await fetch(`${baseUrl}${path}`, { headers })
  if (!response.ok) throw new ApiError('媒体读取失败', `HTTP_${response.status}`)
  return response.blob()
}

export async function uploadFile(path: string, file: File): Promise<{ id: number; mediaType: string }> {
  const form = new FormData()
  form.append('file', file)
  return apiRequest(path, { method: 'POST', body: form })
}
