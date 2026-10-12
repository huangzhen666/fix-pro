export type Diagnostic = { requestId: string; clientGenerated: boolean; status?: number; code: string; operation: string; time: string; clientVersion: string }
export function operation(path: string): string {
  const group = ['auth', 'orders', 'work-orders', 'media', 'customers', 'workers', 'catalog', 'profile'].find((x) => path.split(/[/?]/).includes(x))
  return group ?? 'api'
}
export function safeId(value: unknown): string | undefined { return typeof value === 'string' && /^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/.test(value) ? value : undefined }
export function safeCode(value: unknown, fallback: string): string { return typeof value === 'string' && /^[A-Z0-9_]{1,64}$/.test(value) ? value : fallback }
export function diagnosticText(d: Diagnostic): string {
  return [`${d.clientGenerated ? '客户端参考号' : '请求参考号'}：${d.requestId}`, `时间：${d.time}`, `操作：${d.operation}`, `错误码：${d.code}`, `HTTP：${d.status ?? '未收到响应'}`, `客户端版本：${d.clientVersion}`].join('\n')
}
