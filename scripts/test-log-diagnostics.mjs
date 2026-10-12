// Run production TS request wrappers with controlled browser/WeChat responses.
import assert from 'node:assert/strict'
import fs from 'node:fs'
import vm from 'node:vm'
import ts from 'typescript'

function load(path, dependencies, globals = {}) {
  let source = fs.readFileSync(path, 'utf8')
  source = source.replaceAll('import.meta.env.VITE_API_BASE_URL', "''").replaceAll('import.meta.env.VITE_APP_VERSION', "'test'")
  const code = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText
  const exports = {}
  vm.runInNewContext(code, { exports, require: name => { if (!(name in dependencies)) throw new Error(`Missing stub: ${name}`); return dependencies[name] }, console, Date, Math, Promise, Error, Headers, Response, crypto: { randomUUID: () => 'client-id' }, ...globals }, { filename: path })
  return exports
}
let response = new Response(JSON.stringify({ code: 'FAILED', message: 'failed', requestId: 'server-id' }), { status: 500 })
const notifications = []
let cleared = 0
const browser = { window: { dispatchEvent: event => notifications.push(event.detail) }, CustomEvent: class { constructor(_, options) { this.detail = options.detail } }, fetch: async () => { if (response instanceof Error) throw response; return response } }
const adminD = load('apps/admin-web/src/api/diagnostics.ts', {})
const admin = load('apps/admin-web/src/api/http.ts', { './diagnostics': adminD, '../stores/authStore': { useAuthStore: { getState: () => ({ clearSession: () => cleared++ }) } } }, browser)
await assert.rejects(admin.apiRequest('/api/v1/orders'), e => e.requestId === 'server-id' && e.status === 500 && e.diagnostic.status === 500 && !e.diagnostic.clientGenerated)
response = new Response('<html>bad gateway</html>', { status: 502, headers: { 'X-Request-Id': 'proxy-id' } })
await assert.rejects(admin.apiRequest('/api/v1/orders?phone=secret'), e => e.code === 'INVALID_RESPONSE' && e.requestId === 'proxy-id' && !adminD.diagnosticText(e.diagnostic).includes('secret'))
response = new Error('network error')
await assert.rejects(admin.apiRequest('/api/v1/orders'), e => e.requestId === 'client-id' && e.diagnostic.clientGenerated)
response = new Response('', { status: 401, headers: { 'X-Request-Id': 'blob-id' } })
await assert.rejects(admin.apiBlob('/api/v1/media/123'), e => e.requestId === 'blob-id')
assert.equal(cleared, 1)
assert.equal(notifications.length, 4)

let fail = false
let callbacks
const wx = {
  getAccountInfoSync: () => ({ miniProgram: { version: 'test' } }),
  request: options => { queueMicrotask(() => fail ? options.fail({}) : options.success({ statusCode: 500, data: { code: 'FAILED', message: 'failed', requestId: 'mini-server' }, header: {} })) },
  uploadFile: options => { const task = { onHeadersReceived: fn => { callbacks = fn } }; queueMicrotask(() => { callbacks?.({ header: { 'X-Request-Id': 'upload-id' } }); options.success({ statusCode: 502, data: '<html>bad</html>' }) }); return task },
  downloadFile: options => { const task = { onHeadersReceived: fn => { callbacks = fn } }; queueMicrotask(() => { callbacks?.({ header: { 'X-Request-Id': 'download-id' } }); options.success({ statusCode: 500, tempFilePath: '/tmp/fake' }) }); return task },
}
const miniD = load('apps/wechat-mini/miniprogram/services/diagnostics.ts', {}, { wx })
const sessionStub = { ensureReady: async () => {}, sessionToken: () => 'test-token', sessionEpoch: () => 0, clearSession: () => {}, openLogin: () => {}, rememberFile: () => {}, interactionAllowed: () => true }
const mini = load('apps/wechat-mini/miniprogram/services/request.ts', { './diagnostics': miniD, './session': sessionStub, '../config/env': { getApiBaseUrl: () => '' } }, { wx })
await assert.rejects(mini.request({ url: '/api/v1/orders' }), e => e.requestId === 'mini-server' && e.status === 500)
await assert.rejects(mini.upload('/api/v1/media', 'fake'), e => e.requestId === 'upload-id' && !e.diagnostic.clientGenerated)
await assert.rejects(mini.download('/api/v1/media/1'), e => e.requestId === 'download-id' && !e.diagnostic.clientGenerated)
fail = true
await assert.rejects(mini.request({ url: '/api/v1/orders' }), e => e.code === 'NETWORK_ERROR' && e.diagnostic.clientGenerated)
const realSession = load('apps/wechat-mini/miniprogram/services/session.ts', { './diagnostics': miniD, '../config/env': { getApiBaseUrl: () => '' } }, { wx })
await assert.rejects(realSession.rawAuth('/api/v1/mini/auth/phone/login', { phoneCode: 'secret-sentinel' }), e => e.diagnostic.clientGenerated && !miniD.diagnosticText(e.diagnostic).includes('secret-sentinel'))
fail = false
await assert.rejects(realSession.rawAuth('/api/v1/mini/auth/phone/login'), e => e.requestId === 'mini-server')
console.log('Passed: browser JSON/HTML/network/Blob and WeChat request/upload/download/login diagnostics (10 cases).')
