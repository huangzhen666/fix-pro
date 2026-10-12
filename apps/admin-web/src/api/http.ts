import { operation, safeId, safeCode, type Diagnostic } from './diagnostics'
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
 readonly diagnostic?: Diagnostic
 readonly status?: number

  constructor(message: string, code: string, requestId?: string, diagnostic?: Diagnostic) {
    super(message)
    this.code = code
    this.requestId = requestId
 this.diagnostic = diagnostic
 this.status = diagnostic?.status
 if (diagnostic) window.dispatchEvent(new CustomEvent("fixpro:api-error", { detail: diagnostic }))
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

  const id = headers.get('X-Request-Id')!
  const response = await send(path, { ...init, headers, credentials: 'include' }, id)
  let payload: ApiResponse<T>
  try { payload = await response.json() as ApiResponse<T> } catch {
    throw failure(path, id, response, 'INVALID_RESPONSE', '服务器返回异常，请重试')
  }
  if (!payload || typeof payload !== 'object' || typeof payload.code !== 'string') throw failure(path, id, response, 'INVALID_RESPONSE', '服务器返回异常，请重试')
  if (!response.ok || payload.code !== 'OK') {
    throw failure(path, id, response, safeCode(payload.code, `HTTP_${response.status}`), payload.message || '请求失败', payload.requestId)
  }
  return payload.data
}
function failure(path: string, id: string, response: Response | undefined, code: string, message: string, payloadId?: string): ApiError {
  if (response?.status === 401) useAuthStore.getState().clearSession()
  const serverId = safeId(payloadId) ?? safeId(response?.headers.get('X-Request-Id'))
  const d: Diagnostic = { requestId: serverId ?? id, clientGenerated: !serverId, status: response?.status, code, operation: operation(path), time: new Date().toISOString(), clientVersion: import.meta.env.VITE_APP_VERSION ?? 'development' }
  return new ApiError(message, code, d.requestId, d)
}
async function send(path: string, init: RequestInit, id: string): Promise<Response> {
  try { return await fetch(`${baseUrl}${path}`, init) } catch { throw failure(path, id, undefined, 'NETWORK_ERROR', '网络不可用，请重试') }
}
export async function apiBlob(path: string): Promise<Blob> {
  const id = crypto.randomUUID()
  const response = await send(path, { headers: { Accept: '*/*', 'X-Request-Id': id }, credentials: 'include' }, id)
  if (!response.ok) throw failure(path, id, response, `HTTP_${response.status}`, '媒体读取失败')
  try { return await response.blob() } catch { throw failure(path, id, response, 'MEDIA_READ_FAILED', '媒体读取失败') }
}

export async function uploadFile(path: string, file: File): Promise<{ id: string; mediaType: string; contentType?: string; size?: number }> {
  const form = new FormData()
  form.append('file', file)
  return apiRequest(path, { method: 'POST', body: form })
}
