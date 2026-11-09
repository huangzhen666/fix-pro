'use strict'
// CloudBase HTTP function. Uses only Node built-ins; never trusts a plaintext payment notification.
const crypto = require('node:crypto')
const http = require('node:http')
const cfg = () => ({ appId: process.env.appId, merchantId: process.env.merchantId, serial: process.env.merchantSerialNumber,
  privateKey: process.env.privateKey?.replace(/\\n/g, '\n'), publicKey: process.env.wxPayPublicKey?.replace(/\\n/g, '\n'), publicKeyId: process.env.wxPayPublicKeyId,
  backend: process.env.PAYMENT_BACKEND_URL, secret: process.env.PAYMENT_BRIDGE_SECRET,
  payNotify: process.env.notifyURLPayURL, refundNotify: process.env.notifyURLRefundsURL })
const nonce = () => crypto.randomBytes(16).toString('hex')
const digest = b => crypto.createHash('sha256').update(b).digest('hex')
function signature(secret, method, path, stamp, n, body) { return crypto.createHmac('sha256', secret).update(`${method}\n${path}\n${stamp}\n${n}\n${digest(body)}`).digest('hex') }
function equal(a, b) { return typeof a === 'string' && typeof b === 'string' && a.length === b.length && crypto.timingSafeEqual(Buffer.from(a), Buffer.from(b)) }
function authenticate(c, req, body) {
  const t = req.headers['x-pay-timestamp'], n = req.headers['x-pay-nonce'], sig = req.headers['x-pay-signature']
  return /^\d+$/.test(t || '') && Math.abs(Date.now() / 1000 - Number(t)) <= 300 && typeof n === 'string' && n.length >= 16 && n.length <= 128 && equal(sig, signature(c.secret, req.method, new URL(req.url, 'https://bridge').pathname, t, n, body))
}
function validateConfig(c) {
  for (const k of ['appId', 'merchantId', 'serial', 'privateKey', 'publicKey', 'publicKeyId', 'backend', 'secret', 'payNotify', 'refundNotify']) if (!c[k]) throw new Error(`Missing configuration: ${k}`)
  if (c.secret.length < 32) throw new Error('Bridge secret must contain at least 32 bytes')
  for (const v of [c.backend, c.payNotify, c.refundNotify]) { const u = new URL(v); if (u.protocol !== 'https:' || u.username || u.password || u.search || u.hash || (u.pathname !== '/' && (v === c.backend))) throw new Error('HTTPS URLs without credentials/query/fragment required') }
  crypto.createPrivateKey(c.privateKey); crypto.createPublicKey(c.publicKey)
}
async function backend(c, action, data) {
  const url = new URL(`${c.backend.replace(/\/$/, '')}/api/v1/internal/payment/${action}`)
  const body = JSON.stringify(data), stamp = String(Math.floor(Date.now() / 1000)), n = nonce()
  const r = await fetch(url, { method: 'POST', redirect: 'error', signal: AbortSignal.timeout(8000), headers: { 'content-type': 'application/json', 'x-pay-timestamp': stamp, 'x-pay-nonce': n, 'x-pay-signature': signature(c.secret, 'POST', url.pathname, stamp, n, body) }, body })
  const result = await r.json()
  if (!r.ok || result.code !== 'OK') throw new Error('Business verification failed')
  return result.data
}
class WechatError extends Error { constructor(code) { super('Wechat request failed'); this.code = code } }
async function wechat(c, method, path, data) {
  const body = data ? JSON.stringify(data) : '', stamp = String(Math.floor(Date.now() / 1000)), n = nonce()
  const rsa = crypto.sign('RSA-SHA256', Buffer.from(`${method}\n${path}\n${stamp}\n${n}\n${body}\n`), c.privateKey).toString('base64')
  const authorization = `WECHATPAY2-SHA256-RSA2048 mchid="${c.merchantId}",nonce_str="${n}",timestamp="${stamp}",serial_no="${c.serial}",signature="${rsa}"`
  const r = await fetch(`https://api.mch.weixin.qq.com${path}`, { method, redirect: 'error', signal: AbortSignal.timeout(8000), headers: { Authorization: authorization, Accept: 'application/json', 'Content-Type': 'application/json', 'Wechatpay-Serial': c.publicKeyId }, ...(body ? { body } : {}) })
  const raw = await r.text()
  // Successful close responses can be empty 204. All bodies, including errors, must verify.
  if (!(r.status === 204 && raw === '')) {
    const ts = r.headers.get('wechatpay-timestamp'), rn = r.headers.get('wechatpay-nonce'), sig = r.headers.get('wechatpay-signature')
    if (r.headers.get('wechatpay-serial') !== c.publicKeyId || !ts || !rn || !sig || Math.abs(Date.now() / 1000 - Number(ts)) > 300 || !crypto.verify('RSA-SHA256', Buffer.from(`${ts}\n${rn}\n${raw}\n`), c.publicKey, Buffer.from(sig, 'base64'))) throw new Error('Invalid Wechat response signature')
  }
  const result = raw ? JSON.parse(raw) : {}
  if (!r.ok) throw new WechatError(result.code)
  return result
}
function paymentEvent(c, q) {
  if (q.appid !== c.appId || q.mchid !== c.merchantId || q.amount?.currency !== 'CNY') throw new Error('Wechat identity mismatch')
  return { kind: 'PAYMENT', channel: 'WECHAT', outTradeNo: q.out_trade_no, transactionId: q.transaction_id || `CLOSED_${q.out_trade_no}`, amount: q.amount.total, total: q.amount.total, currency: q.amount.currency, appId: q.appid, merchantId: q.mchid, status: q.trade_state === 'REFUND' ? 'SUCCESS' : q.trade_state }
}
async function query(c, trade, expired = false) {
  if (typeof trade !== 'string' || !/^P[a-f0-9]{24}$/.test(trade)) throw new Error('Invalid trade number')
  const path = `/v3/pay/transactions/out-trade-no/${encodeURIComponent(trade)}?mchid=${encodeURIComponent(c.merchantId)}`
  let q
  try { q = await wechat(c, 'GET', path) } catch (e) {
    // A never-created prepay still needs authoritative business amount for a close event.
    if (e.code === 'ORDER_NOT_EXIST' && expired) return { missing: true, outTradeNo: trade, channel: 'WECHAT' }
    throw e
  }
  if (expired && q.trade_state === 'NOTPAY') {
    await wechat(c, 'POST', `/v3/pay/transactions/out-trade-no/${encodeURIComponent(trade)}/close`, { mchid: c.merchantId })
    q = await wechat(c, 'GET', path)
  }
  return paymentEvent(c, q)
}
async function refund(c, data, create = true) {
  if (!/^R[a-f0-9]{24}$/.test(data.out_refund_no || '')) throw new Error('Invalid refund number')
  let q
  try { q = await wechat(c, 'GET', `/v3/refund/domestic/refunds/${encodeURIComponent(data.out_refund_no)}`) }
  catch (e) { if (!create || e.code !== 'RESOURCE_NOT_EXISTS') throw e; q = await wechat(c, 'POST', '/v3/refund/domestic/refunds', { ...data, notify_url: c.refundNotify }) }
  const pay = await query(c, q.out_trade_no)
  if (pay.status !== 'SUCCESS' || q.transaction_id !== pay.transactionId || q.amount?.total !== pay.total || q.amount?.currency !== 'CNY') throw new Error('Refund identity mismatch')
  return { ...pay, kind: 'REFUND', outRefundNo: q.out_refund_no, refundId: q.refund_id, amount: q.amount.refund, status: ({ CLOSED: 'CLOSED', ABNORMAL: 'ABNORMAL', SUCCESS: 'SUCCESS', PROCESSING: 'PROCESSING' })[q.status] || 'ABNORMAL' }
}
async function prepay(c, ticket) {
  const data = await backend(c, 'authorize', { ticket })
  // OpenID and amount come only from Go's authenticated customer binding and order snapshot.
  const {prepay_id: cachedPrepay, ...order} = data
  const q = cachedPrepay ? {prepay_id:cachedPrepay} : await wechat(c, 'POST', '/v3/pay/transactions/jsapi', { ...order, appid: c.appId, mchid: c.merchantId, notify_url: c.payNotify })
  if(!cachedPrepay) await backend(c, 'prepay', {ticket,prepayId:q.prepay_id})
  if (typeof q.prepay_id !== 'string') throw new Error('Invalid prepay response')
  const timeStamp = String(Math.floor(Date.now() / 1000)), nonceStr = nonce(), pkg = `prepay_id=${q.prepay_id}`
  const paySign = crypto.sign('RSA-SHA256', Buffer.from(`${c.appId}\n${timeStamp}\n${nonceStr}\n${pkg}\n`), c.privateKey).toString('base64')
  return { timeStamp, nonceStr, package: pkg, signType: 'RSA', paySign }
}
function callbackResource(c, body, headers, raw) {
  // Integration Center can forward plaintext. Plaintext only supplies a lookup key;
  // an independently authenticated Wechat query determines every accounting field.
  if (!body.resource?.ciphertext) return body.resource?.plaintext || body.resource || body
  const ts = headers['wechatpay-timestamp'], n = headers['wechatpay-nonce'], sig = headers['wechatpay-signature']
  if (headers['wechatpay-serial'] !== c.publicKeyId || !ts || !n || !sig || Math.abs(Date.now() / 1000 - Number(ts)) > 300 || !crypto.verify('RSA-SHA256', Buffer.from(`${ts}\n${n}\n${raw}\n`), c.publicKey, Buffer.from(sig, 'base64'))) throw new Error('Invalid callback signature')
  const key = process.env.apiV3Key
  if (!key || Buffer.byteLength(key) !== 32) throw new Error('Invalid APIv3 key')
  const r = body.resource, encrypted = Buffer.from(r.ciphertext, 'base64'), d = crypto.createDecipheriv('aes-256-gcm', Buffer.from(key), Buffer.from(r.nonce))
  d.setAuthTag(encrypted.subarray(-16)); d.setAAD(Buffer.from(r.associated_data || ''))
  return JSON.parse(Buffer.concat([d.update(encrypted.subarray(0, -16)), d.final()]).toString())
}
function createServer(c = cfg()) {
  validateConfig(c)
  return http.createServer(async (req, res) => {
    let raw = ''; let tooLarge = false
    req.on('data', chunk => { raw += chunk; if (Buffer.byteLength(raw) > 65536) { tooLarge = true; req.destroy() } })
    req.on('end', async () => {
      if (tooLarge) return
      const path = new URL(req.url, 'https://bridge').pathname
      const send = (status, data, signed = false) => {
        const body = JSON.stringify(data); res.setHeader('Content-Type', 'application/json')
        if (signed) { const stamp = String(Math.floor(Date.now() / 1000)); const n = req.headers['x-pay-nonce']; res.setHeader('X-Pay-Timestamp', stamp); res.setHeader('X-Pay-Signature', signature(c.secret, 'RESPONSE', path, stamp, n, body)) }
        res.writeHead(status); res.end(body)
      }
      try {
        if (req.method !== 'POST') return send(405, { code: 'METHOD_NOT_ALLOWED' })
        const data = JSON.parse(raw || '{}')
        if (path === '/wx-pay/prepay') {
          const p = await prepay(c, data.ticket)
          return send(200, { code: 0, data: { data: p } })
        }
        if (path.startsWith('/internal/')) {
          if (!authenticate(c, req, raw)) return send(401, { code: 'UNAUTHORIZED' })
          let event
          if (path === '/internal/query') {
            event = await query(c, data.out_trade_no, data.expired === true)
            // Never invent a CLOSED amount when Wechat has no record. Go supplies authoritative fields.
          } else if (path === '/internal/refund') event = await refund(c, data)
          else return send(404, { code: 'NOT_FOUND' })
          return send(200, event, true)
        }
        if (['/wechatpay/order', '/wechatpay/refund', '/wx-pay/unifiedOrderTrigger', '/wx-pay/refundTrigger'].includes(path)) {
          const resource = callbackResource(c, data, req.headers, raw)
          const isRefund = path.includes('refund')
          const event = isRefund ? await refund(c, { out_refund_no: resource.out_refund_no }, false) : await query(c, resource.out_trade_no)
          if (event.status === 'SUCCESS' || event.status === 'CLOSED' || event.kind === 'REFUND' && event.status === 'ABNORMAL') await backend(c, 'events', event)
          else throw new Error('Channel result is not final')
          return send(200, { code: 'SUCCESS' })
        }
        return send(404, { code: 'NOT_FOUND' })
      } catch { return send(502, { code: 'PAYMENT_BRIDGE_ERROR', message: '支付渠道处理中，请查询订单结果' }) }
    })
  })
}
if (require.main === module) createServer().listen(Number(process.env.PORT || 9000), '0.0.0.0')
module.exports = { signature, authenticate, equal, validateConfig, paymentEvent, callbackResource, createServer, prepay, query, refund, wechat }
