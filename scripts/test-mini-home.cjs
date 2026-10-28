const test = require('node:test')
const assert = require('node:assert/strict')
const fs = require('node:fs')
const vm = require('node:vm')
const path = require('node:path')
const ts = require('typescript')
const root = path.resolve(__dirname, '..')

function compile(relative, imports, globals = {}) {
  const exports = {}
  const code = ts.transpileModule(fs.readFileSync(path.join(root, relative), 'utf8'), { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 } }).outputText
  vm.runInNewContext(code, { exports, require: name => {
    const key = name.split('/').at(-1)
    if (!(key in imports)) throw new Error(`Unexpected dependency: ${name}`)
    return imports[key]
  }, ...globals })
  return exports
}
const service = (id, overrides = {}) => ({ id, name: `服务${id}`, price: 11800, unit: '次', coverImageUrl: '/cover', galleryImageUrls: [], publishedVersion: 1, ...overrides })
function deferred() { let resolve, reject; const promise = new Promise((yes, no) => { resolve = yes; reject = no }); return { promise, resolve, reject } }
function harness({ groups = [], list = async () => groups, getCart = async () => ({ itemCount: 3 }), loggedIn = true } = {}) {
  let page, epoch = 1, reset
  const storage = new Map(), navigations = []
  const presentation = compile('apps/wechat-mini/miniprogram/services/presentation.ts', { env: { getApiBaseUrl: () => 'http://api.test' } })
  compile('apps/wechat-mini/miniprogram/pages/home/index.ts', {
    catalog: { listCategoryGroups: list }, cart: { getCart }, presentation,
    session: { hasSession: () => loggedIn, sessionEpoch: () => epoch, onSessionReset: fn => { reset = fn } },
  }, {
    Page: options => { page = options }, getCurrentPages: () => [page],
    wx: { setStorageSync: (k, v) => storage.set(k, v), navigateTo: o => navigations.push(o.url), switchTab: o => navigations.push(o.url), setNavigationBarTitle() {} },
  })
  page.route = 'pages/home/index'
  page.setData = update => {
    for (const [key, value] of Object.entries(update)) {
      const keys = key.replace(/\[(\d+)\]/g, '.$1').split('.'); let target = page.data
      for (const name of keys.slice(0, -1)) target = target[name]
      target[keys.at(-1)] = value
    }
  }
  return { page, presentation, storage, navigations, reset: () => { epoch++; reset() } }
}

test('目录成功时，私有购物车失败不影响公开服务展示', async () => {
  const { page } = harness({ groups: [{ id: '1001', name: '精准测漏', services: [service('1')] }], getCart: async () => { throw Error('session expired') } })
  await Promise.all([page.load(), page.refreshCart()])
  assert.equal(page.data.error, '')
  assert.equal(page.data.recommended[0].priceText, '118.00')
  assert.equal(page.data.cartCount, 0)
})
test('目录失败与真正空目录区分，重试后可恢复', async () => {
  let failed = true
  const { page } = harness({ list: async () => { if (failed) throw Error('network'); return [] } })
  await page.load(); assert.match(page.data.error, /加载失败/); assert.equal(page.data.loading, false)
  failed = false; await page.load(); assert.equal(page.data.error, ''); assert.equal(page.data.recommended.length, 0)
})
test('游客不请求私有购物车，身份切换后不回写旧数量', async () => {
  let calls = 0
  const guest = harness({ loggedIn: false, getCart: async () => { calls++; return { itemCount: 5 } } })
  await guest.page.refreshCart(); assert.equal(calls, 0)
  const pending = deferred(), active = harness({ getCart: () => pending.promise })
  const loading = active.page.refreshCart(); active.reset(); pending.resolve({ itemCount: 99 }); await loading
  assert.equal(active.page.data.cartCount, 0)
})
test('快速刷新与离开页面后，旧响应不会覆盖当前页面', async () => {
  const requests = [deferred(), deferred()]; let i = 0
  const { page } = harness({ list: () => requests[i++].promise })
  const first = page.load(), second = page.load()
  requests[1].resolve([{ id: 'x', name: '新类别', services: [service('new')] }]); await second
  requests[0].resolve([{ id: 'x', name: '旧类别', services: [service('old')] }]); await first
  assert.equal(page.data.recommended[0].id, 'new')
  const pending = deferred(), other = harness({ list: () => pending.promise })
  const load = other.page.load(); other.page.onUnload(); pending.resolve([]); await load
  assert.equal(other.page.data.loading, true)
})
test('类别改序不改变图标，未知类别回退，八类入口与真实价格保留', async () => {
  const groups = Array.from({ length: 9 }, (_, i) => ({ id: String(i + 1001), name: i ? `类别${i}` : '精准测漏', services: [service(String(i), { price: i ? 99999999 : 0, description: '', coverImageUrl: i ? 'https://media.test/a.png' : '' })] }))
  const { page, presentation } = harness({ groups })
  const before = presentation.categoryIcon(groups[0].id, groups[0].name)
  groups.reverse(); assert.equal(presentation.categoryIcon('1001', '精准测漏'), before)
  assert.match(presentation.categoryIcon('unknown', '未知分类'), /grid.svg$/)
  await page.load(); assert.equal(page.data.categories.length, 8); assert.equal(page.data.recommended.length, 4)
  assert.equal(page.data.recommended[0].priceText, '999999.99')
  assert.equal(page.data.recommended[0].coverImageUrl, 'https://media.test/a.png')
  page.imageError({ currentTarget: { dataset: { id: page.data.recommended[0].id } } }); assert.equal(page.data.recommended[0].imageFailed, true)
  const zero = harness({ groups: [groups.at(-1)] }); await zero.page.load()
  assert.equal(zero.page.data.recommended[0].priceText, '0.00'); assert.equal(zero.page.data.recommended[0].imageFailed, true)
})
test('推荐服务去重，分类与详情跳转继续传真实 ID', async () => {
  const { page, storage, navigations } = harness({ groups: [{ id: 'a', name: '测漏', services: [service('one')] }, { id: 'b', name: '维修', services: [service('one')] }] })
  await page.load(); assert.equal(page.data.recommended.length, 1)
  page.openCategory({ currentTarget: { dataset: { id: 'b' } } }); assert.equal(storage.get('fixpro.selectedCategory'), 'b')
  page.openService({ currentTarget: { dataset: { id: 'one' } } }); assert.equal(navigations.at(-1), '/pages/services/detail?id=one')
})
