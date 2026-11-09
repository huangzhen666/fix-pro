'use strict'
const { test } = require('node:test')
const assert = require('node:assert/strict')
const crypto = require('node:crypto')
const { signature, authenticate, validateConfig, paymentEvent, callbackResource, prepay, wechat } = require('./index.cjs')
const {privateKey, publicKey}=crypto.generateKeyPairSync('rsa',{modulusLength:2048})
const cfg={appId:'app',merchantId:'merchant',serial:'serial',privateKey,publicKey,publicKeyId:'PUB_KEY_ID_test',secret:'a'.repeat(32),backend:'https://api.example.test',payNotify:'https://callback.example.test/wechatpay/order',refundNotify:'https://callback.example.test/wechatpay/refund'}
function signedResponse(data,status=200,serial=cfg.publicKeyId) {const raw=JSON.stringify(data),ts=String(Math.floor(Date.now()/1000)),n='response-nonce';const sig=crypto.sign('RSA-SHA256',Buffer.from(`${ts}\n${n}\n${raw}\n`),privateKey).toString('base64');return new Response(raw,{status,headers:{'wechatpay-timestamp':ts,'wechatpay-nonce':n,'wechatpay-serial':serial,'wechatpay-signature':sig}})}
test('bridge signatures bind method, path, timestamp, nonce and body',()=>{
 const stamp=String(Math.floor(Date.now()/1000)),n='b'.repeat(20),body='{"amount":100}'
 const req={method:'POST',url:'/internal/query',headers:{'x-pay-timestamp':stamp,'x-pay-nonce':n,'x-pay-signature':signature(cfg.secret,'POST','/internal/query',stamp,n,body)}}
 assert.equal(authenticate(cfg,req,body),true);assert.equal(authenticate(cfg,req,'{"amount":1}'),false)
 assert.equal(authenticate(cfg,{...req,url:'/internal/refund'},body),false)
 const old=String(Math.floor(Date.now()/1000)-1000);assert.equal(authenticate(cfg,{...req,headers:{...req.headers,'x-pay-timestamp':old,'x-pay-signature':signature(cfg.secret,'POST','/internal/query',old,n,body)}},body),false)
})
test('config requires secrets, keys and HTTPS',()=>{
 const pemCfg={...cfg,privateKey:privateKey.export({type:'pkcs8',format:'pem'}),publicKey:publicKey.export({type:'spki',format:'pem'})}
 assert.doesNotThrow(()=>validateConfig(pemCfg));assert.throws(()=>validateConfig({...pemCfg,backend:'http://localhost'}));assert.throws(()=>validateConfig({...pemCfg,secret:'short'}))
})
test('payment event rejects unrelated merchant/app/currency',()=>{
 const data={appid:'app',mchid:'merchant',out_trade_no:'P'+'a'.repeat(24),transaction_id:'wx',amount:{total:100,currency:'CNY'},trade_state:'SUCCESS'}
 assert.equal(paymentEvent(cfg,data).amount,100);assert.equal(paymentEvent(cfg,{...data,trade_state:'REFUND'}).status,'SUCCESS')
 assert.throws(()=>paymentEvent(cfg,{...data,appid:'other'}));assert.throws(()=>paymentEvent(cfg,{...data,mchid:'other'}))
})
test('prepay takes OpenID and amount only from authenticated Go response',async()=>{
 const original=global.fetch;const calls=[]
 global.fetch=async(url,options)=>{calls.push([String(url),JSON.parse(options.body)]);if(String(url).startsWith(cfg.backend))return Response.json({code:'OK',data:{out_trade_no:'P'+'a'.repeat(24),amount:{total:100,currency:'CNY'},payer:{openid:'trusted-openid'},description:'test',time_expire:'2030-01-01T00:00:00Z'}});return signedResponse({prepay_id:'wx-prepay'})}
 try {const p=await prepay(cfg,'opaque-ticket');assert.equal(calls[0][1].ticket,'opaque-ticket');assert.equal(calls[1][1].amount.total,100);assert.equal(calls[1][1].payer.openid,'trusted-openid');assert.equal(p.package,'prepay_id=wx-prepay');assert.equal(crypto.verify('RSA-SHA256',Buffer.from(`${cfg.appId}\n${p.timeStamp}\n${p.nonceStr}\n${p.package}\n`),publicKey,Buffer.from(p.paySign,'base64')),true)} finally{global.fetch=original}
})
test('channel responses with wrong signature or public key ID are rejected',async()=>{
 const original=global.fetch
 try{global.fetch=async()=>signedResponse({trade_state:'SUCCESS'},200,'WRONG_KEY');await assert.rejects(()=>wechat(cfg,'GET','/v3/test'))
 global.fetch=async()=>new Response('{"trade_state":"SUCCESS"}',{status:200});await assert.rejects(()=>wechat(cfg,'GET','/v3/test'))}finally{global.fetch=original}
})
test('encrypted notifications require valid Wechat signature and AES-GCM authentication',()=>{
 const key='k'.repeat(32),resource={out_trade_no:'P'+'a'.repeat(24)},n='123456789012',aad='transaction',cipher=crypto.createCipheriv('aes-256-gcm',Buffer.from(key),Buffer.from(n));cipher.setAAD(Buffer.from(aad));const encrypted=Buffer.concat([cipher.update(JSON.stringify(resource)),cipher.final(),cipher.getAuthTag()]);const body={resource:{nonce:n,associated_data:aad,ciphertext:encrypted.toString('base64')}},raw=JSON.stringify(body),ts=String(Math.floor(Date.now()/1000)),rn='notification-nonce';const headers={'wechatpay-timestamp':ts,'wechatpay-nonce':rn,'wechatpay-serial':cfg.publicKeyId,'wechatpay-signature':crypto.sign('RSA-SHA256',Buffer.from(`${ts}\n${rn}\n${raw}\n`),privateKey).toString('base64')}
 const old=process.env.apiV3Key;process.env.apiV3Key=key
 try{assert.deepEqual(callbackResource(cfg,body,headers,raw),resource);assert.throws(()=>callbackResource(cfg,body,{...headers,'wechatpay-signature':'wrong'},raw))}finally{if(old===undefined)delete process.env.apiV3Key;else process.env.apiV3Key=old}
})
test('forged plaintext success is only a query hint and cannot trigger business receipt',async()=>{
 const {createServer}=require('./index.cjs'),original=global.fetch
 const c={...cfg,privateKey:privateKey.export({type:'pkcs8',format:'pem'}),publicKey:publicKey.export({type:'spki',format:'pem'})}
 const server=createServer(c);await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));let eventCalls=0
 global.fetch=async(url,options)=>{if(String(url).startsWith('http://127.0.0.1:'))return original(url,options);if(String(url).startsWith(c.backend)){eventCalls++;return Response.json({code:'OK',data:{accepted:true}})}return signedResponse({appid:'app',mchid:'merchant',out_trade_no:'P'+'a'.repeat(24),amount:{total:100,currency:'CNY'},trade_state:'NOTPAY'})}
 try{const r=await global.fetch(`http://127.0.0.1:${server.address().port}/wechatpay/order`,{method:'POST',body:JSON.stringify({resource:{out_trade_no:'P'+'a'.repeat(24),trade_state:'SUCCESS',amount:{total:1,currency:'CNY'}}})});assert.equal(r.status,502);assert.equal(eventCalls,0)}finally{global.fetch=original;await new Promise(resolve=>server.close(resolve))}
})
