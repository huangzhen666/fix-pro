import { getApiBaseUrl } from '../config/env'
import { ensureReady, sessionToken, sessionEpoch, clearSession, openLogin, rememberFile, interactionAllowed } from './session'
import { DiagnosticError as ApiError, makeDiagnostic, showErrorDetails } from './diagnostics'
export interface ApiResponse<T> { code: string; message: string; data: T; requestId: string }
export { DiagnosticError as ApiError } from './diagnostics'
function requestId() { return `${Date.now()}-${Math.random().toString(16).slice(2)}` }
function expired(status: number) { if (status === 401) { clearSession(); openLogin() } }
function failure(path: string, id: string, status: number | undefined, body: Partial<ApiResponse<unknown>> | undefined, header: Record<string, unknown> | undefined, fallback: string): ApiError {
  const d = makeDiagnostic(path, id, status, body, header)
  return new ApiError(d.code, body?.message || fallback, d.requestId, status, d)
}
export function showApiError(error: unknown) {
  if (error instanceof ApiError && error.diagnostic) wx.showModal({ title: '操作未完成', content: error.message, confirmText: '错误详情', cancelText: '关闭', success: r => { if (r.confirm) showErrorDetails(error.diagnostic!) } })
  else wx.showToast({ title: error instanceof Error ? error.message : '操作失败，请重试', icon: 'none' })
}
export async function request<T>(options: WechatMiniprogram.RequestOption): Promise<T> {
  const privateRequest = options.url.startsWith('/api/v1/mini/')
  if (privateRequest) { try { await ensureReady() } catch (e) { if (!sessionToken()) openLogin(); throw e } }
  const token = sessionToken(); const current = sessionEpoch(); const id = requestId()
  return new Promise((resolve, reject) => wx.request<ApiResponse<T>>({ ...options, url: getApiBaseUrl() + options.url,
    header: { Accept: 'application/json', 'Content-Type': 'application/json', ...(token && privateRequest ? { Authorization: `Bearer ${token}` } : {}), ...(privateRequest && interactionAllowed(options.method ?? 'GET') ? { 'X-Customer-Interaction': '1' } : {}), ...(options.header ?? {}), 'X-Request-Id': id },
    success(r) {
      if (privateRequest && current !== sessionEpoch()) { reject(new Error('登录状态已变化')); return }
      if (privateRequest) expired(r.statusCode)
      const b = typeof r.data === 'object' && r.data ? r.data : undefined
      if (r.statusCode >= 200 && r.statusCode < 300 && b?.code === 'OK') resolve(b.data)
      else reject(failure(options.url, id, r.statusCode, b, r.header, '请求失败'))
    }, fail() { reject(failure(options.url, id, undefined, undefined, undefined, '网络不可用，请重试')) }
  }))
}
export async function upload<T>(path: string, filePath: string): Promise<T> {
  await ensureReady(); const token = sessionToken(); const current = sessionEpoch(); const id = requestId()
  return new Promise((resolve, reject) => {
 let responseHeaders: Record<string, unknown> | undefined
 const task = wx.uploadFile({ url: getApiBaseUrl() + path, filePath, name: 'file', header: { Authorization: `Bearer ${token}`, 'X-Request-Id': id }, success(r) {
    if (current !== sessionEpoch()) { reject(new Error('登录状态已变化')); return }
    expired(r.statusCode)
    let b: ApiResponse<T> | undefined
    try { const value = JSON.parse(r.data); if (value && typeof value === 'object') b = value } catch { /* preserve HTTP/reference information */ }
    if (r.statusCode >= 200 && r.statusCode < 300 && b?.code === 'OK') resolve(b.data)
    else reject(failure(path, id, r.statusCode, b, responseHeaders, '图片上传失败，请重试'))
  }, fail() { reject(failure(path, id, undefined, undefined, undefined, '图片上传失败，请重试')) } })
 task.onHeadersReceived?.(r => { responseHeaders = r.header })
 })
}
export async function download(path: string): Promise<string> {
  await ensureReady(); const token = sessionToken(); const current = sessionEpoch(); const id = requestId()
  return new Promise((resolve, reject) => {
 let responseHeaders: Record<string, unknown> | undefined
 const task = wx.downloadFile({ url: getApiBaseUrl() + path, header: { Authorization: `Bearer ${token}`, 'X-Request-Id': id }, success(r) {
    if (current !== sessionEpoch()) { wx.getFileSystemManager().unlink({ filePath: r.tempFilePath, fail() {} }); reject(new Error('登录状态已变化')); return }
    expired(r.statusCode); if (r.statusCode >= 200 && r.statusCode < 300) { rememberFile(r.tempFilePath); resolve(r.tempFilePath) } else reject(failure(path, id, r.statusCode, undefined, responseHeaders, '图片加载失败'))
  }, fail() { reject(failure(path, id, undefined, undefined, undefined, '图片加载失败')) } })
 task.onHeadersReceived?.(r => { responseHeaders = r.header })
 })
}
