export type Diagnostic = { requestId: string; clientGenerated: boolean; status?: number; code: string; operation: string; time: string; clientVersion: string }
export function safeId(v: unknown): string | undefined { return typeof v === 'string' && /^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/.test(v) ? v : undefined }
export function safeCode(v: unknown, fallback: string): string { return typeof v === 'string' && /^[A-Z0-9_]{1,64}$/.test(v) ? v : fallback }
export function operation(path: string): string { return ['auth', 'orders', 'work-orders', 'media', 'profile'].find(x => path.split(/[/?]/).includes(x)) ?? 'api' }
export function diagnosticText(d: Diagnostic): string { return [`${d.clientGenerated ? '客户端参考号' : '请求参考号'}：${d.requestId}`, `时间：${d.time}`, `操作：${d.operation}`, `错误码：${d.code}`, `HTTP：${d.status ?? '未收到响应'}`, `客户端版本：${d.clientVersion}`].join('\n') }
export function showErrorDetails(d: Diagnostic) { wx.showModal({ title: '错误详情', content: diagnosticText(d), confirmText: '复制详情', cancelText: '关闭', success: r => { if (r.confirm) wx.setClipboardData({ data: diagnosticText(d) }) } }) }

export class DiagnosticError extends Error {
 constructor(public readonly code: string, message: string, public readonly requestId?: string, public readonly status?: number, public readonly diagnostic?: Diagnostic) { super(message) }
}
export function makeDiagnostic(path: string, id: string, status?: number, body?: { code?: unknown; requestId?: unknown }, header?: Record<string, unknown>): Diagnostic {
 const serverId = safeId(body?.requestId) ?? safeId(header?.['X-Request-Id']) ?? safeId(header?.['x-request-id'])
 return { requestId: serverId ?? id, clientGenerated: !serverId, status, code: safeCode(body?.code, status === undefined ? 'NETWORK_ERROR' : `HTTP_${status}`), operation: operation(path), time: new Date().toISOString(), clientVersion: wx.getAccountInfoSync().miniProgram.version || 'development' }
}
