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
  const headers = new Headers(init.headers)
  headers.set('Accept', 'application/json')
  headers.set('X-Request-Id', crypto.randomUUID())
  if (init.body && !(init.body instanceof FormData)) headers.set('Content-Type', 'application/json')
  if (init.method && !['GET', 'HEAD', 'OPTIONS'].includes(init.method.toUpperCase())) {
    const csrf = document.cookie.split('; ').find((item) => item.startsWith('fixpro_admin_csrf='))?.split('=').slice(1).join('=')
    if (csrf) headers.set('X-CSRF-Token', decodeURIComponent(csrf))
  }

  const response = await fetch(`${baseUrl}${path}`, { ...init, headers, credentials: 'include' })
  const payload = (await response.json()) as ApiResponse<T>
  if (!response.ok || payload.code !== 'OK') {
    if (response.status === 401) useAuthStore.getState().clearSession()
    throw new ApiError(payload.message || '请求失败', payload.code, payload.requestId)
  }
  return payload.data
}

export async function apiBlob(path: string): Promise<Blob> {
  const headers = new Headers({ Accept: '*/*', 'X-Request-Id': crypto.randomUUID() })
  const response = await fetch(`${baseUrl}${path}`, { headers, credentials: 'include' })
  if (!response.ok) throw new ApiError('媒体读取失败', `HTTP_${response.status}`)
  return response.blob()
}

export async function uploadFile(path: string, file: File): Promise<{ id: string; mediaType: string; contentType?: string; size?: number }> {
  const form = new FormData()
  form.append('file', file)
  return apiRequest(path, { method: 'POST', body: form })
}
