import { DiagnosticError, makeDiagnostic } from './diagnostics'
import { getApiBaseUrl } from '../config/env'
export interface CustomerProfile { id: string; displayName: string; status: string; mobileMasked: string; avatarMediaId: string | null; avatarUrl: string | null; version: number }
interface StoredSession { token: string; expiresAt: string; customer: CustomerProfile }
const key = 'fixpro.customer.session'
let value: StoredSession | undefined
let verifying: Promise<void> | undefined
let verified = false
let epoch = 0
let foreground = true
let loginOpening = false
const files = new Set<string>()
const resets = new Set<() => void>()
export function onSessionReset(fn: () => void) { resets.add(fn) }
export class SessionError extends DiagnosticError {}
export const sessionEpoch = () => epoch
export const isForeground = () => foreground
export function setForeground(v: boolean) { foreground = v; if (v) verified = false }
export const getCustomer = () => verified ? value?.customer : undefined
export const hasSession = () => !!value && Date.parse(value.expiresAt) > Date.now()
export const sessionToken = () => value?.token ?? ''
export const scopedKey = (name: string) => `fixpro.customer.${value?.customer.id ?? 'guest'}.${name}`
export function rememberFile(path: string) { files.add(path) }
export function clearSession() {
  wx.disableAlertBeforeUnload?.()
  for (const reset of resets) reset()
  epoch++; verified = false; value = undefined; verifying = undefined
  wx.removeStorageSync(key)
  for (const k of wx.getStorageInfoSync().keys) if (k.startsWith('fixpro.customer.') || k === 'profile.read-order-shortcuts' || k === 'fixpro.checkoutCartItemIds' || k === 'fixpro.selectedAddress') wx.removeStorageSync(k)
  for (const path of files) wx.getFileSystemManager().unlink({ filePath: path, fail() {} })
  files.clear()
  for (const page of getCurrentPages()) {
    const p = page as any
    if (p.route?.startsWith('pages/profile') || /pages\/(cart|checkout|addresses|orders)\//.test(p.route ?? '')) {
      p.__customerLoaded = false
      const reset: any = {}
      for (const [name, v] of Object.entries(p.data ?? {})) {
        if (Array.isArray(v)) reset[name] = []
        else if (typeof v === 'string') reset[name] = ''
        else if (typeof v === 'number') reset[name] = 0
        else if (typeof v === 'boolean') reset[name] = false
        else if (v && typeof v === 'object') reset[name] = name === 'cart' ? { items: [], itemCount: 0, totalAmount: 0 } : null
      }
      p.setData(reset)
    }
  }
}
export function bootstrap() {
  wx.removeStorageSync('fixpro.accessToken'); wx.removeStorageSync('fixpro.role'); wx.removeStorageSync('profile.read-order-shortcuts')
  const stored = wx.getStorageSync(key) as StoredSession | undefined
  if (stored?.token && stored.customer && Date.parse(stored.expiresAt) > Date.now()) value = stored
  else clearSession()
}
export function rawAuth<T>(path: string, data?: object): Promise<T> {
  const token = sessionToken()
  const id = `${Date.now()}-${Math.random().toString(16).slice(2)}`
  return new Promise((resolve, reject) => wx.request({ url: getApiBaseUrl() + path, method: data ? 'POST' : 'GET', data, header: { 'X-Request-Id': id, 'Content-Type': 'application/json', ...(token ? { Authorization: `Bearer ${token}` } : {}) }, success(r) {
    const body = typeof r.data === 'object' && r.data ? r.data as any : undefined
    if (r.statusCode >= 200 && r.statusCode < 300 && body?.code === 'OK') resolve(body.data as T)
    else { const d = makeDiagnostic(path, id, r.statusCode, body, r.header); reject(new SessionError(d.code, body?.message ?? '请求失败', d.requestId, r.statusCode, d)) }
  }, fail() { const d = makeDiagnostic(path, id); reject(new SessionError(d.code, '网络不可用，请重试', d.requestId, undefined, d)) } }))
}
export function wxLogin(): Promise<string> { return new Promise((resolve, reject) => wx.login({ success: r => r.code ? resolve(r.code) : reject(new Error('无法取得微信登录凭证')), fail: () => reject(new Error('微信登录校验失败')) })) }
export async function ensureReady(): Promise<void> {
  if (!hasSession()) { if (value) clearSession(); throw new SessionError('CUSTOMER_SESSION_INVALID', '请先授权手机号登录') }
  if (verified) return
  if (!verifying) {
    const current = epoch
    verifying = (async () => {
      try {
        const loginCode = await wxLogin()
        const result = await rawAuth<{ customer: CustomerProfile; expiresAt: string }>('/api/v1/mini/auth/session/verify', { loginCode })
        if (epoch !== current || !value) throw new Error('登录状态已变化')
        value = { ...value, customer: result.customer, expiresAt: result.expiresAt }; verified = true; wx.setStorageSync(key, value)
      } catch (e) {
        if (epoch === current && e instanceof SessionError && ['CUSTOMER_SESSION_INVALID', 'CUSTOMER_WECHAT_CONTEXT_CHANGED'].includes(e.code)) clearSession()
        throw e
      } finally { if (epoch === current) verifying = undefined }
    })()
  }
  return verifying
}
export async function phoneLogin(phoneCode: string) {
  const current = epoch
  const loginCode = await wxLogin()
  const out = await rawAuth<{ accessToken: string; expiresAt: string; customer: CustomerProfile }>('/api/v1/mini/auth/phone/login', { phoneCode, loginCode })
  if (current !== epoch) throw new Error('登录状态已变化，请重试')
  clearSession(); value = { token: out.accessToken, expiresAt: out.expiresAt, customer: out.customer }; verified = true; wx.setStorageSync(key, value)
}
export function updateCustomer(customer: CustomerProfile) { if (value) { value = { ...value, customer }; wx.setStorageSync(key, value) } }
export function openLogin(returnUrl?: string) {
  if (loginOpening) return
  const pages = getCurrentPages(); const current = pages[pages.length - 1] as any
  if (current?.route === 'pages/login/index') return
  const query = Object.entries(current?.options ?? {}).map(([k, v]) => `${encodeURIComponent(k)}=${encodeURIComponent(String(v))}`).join('&')
  const target = returnUrl ?? (current ? `/${current.route}${query ? '?' + query : ''}` : '/pages/profile/index')
  loginOpening = true
  wx.navigateTo({ url: `/pages/login/index?returnUrl=${encodeURIComponent(target)}`, complete() { loginOpening = false } })
}
export function interactionAllowed(method: string) {
  if (!foreground) return false
  if (method !== 'GET') return true
  const pages = getCurrentPages(); return /^pages\/(profile|cart|checkout|addresses|orders)\//.test(pages[pages.length - 1]?.route ?? '')
}
