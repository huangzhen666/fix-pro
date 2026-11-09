// Fixture-only page regressions: no live data or orders are changed.
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs'), vm = require('node:vm'), ts = require('typescript');
const key = 'fixpro.checkoutCartItemIds';
const event = (id, delta) => ({ currentTarget: { dataset: { id, delta } } });
const row = (id, overrides = {}) => ({ id, skuId: 'sku-' + id, name: '服务' + id, available: true, quantity: 1, unit: '次', unitPrice: 100, subtotal: 100, coverImageUrl: '', demandDescription: '上门前联系', demandMedia: [{ id: 'media-' + id, url: '/private/photo', mediaType: 'IMAGE' }], needsConfirmation: false, currentVersion: 2, ...overrides });
const cart = items => ({ items, itemCount: items.reduce((n, i) => n + i.quantity, 0), totalAmount: items.reduce((n, i) => n + i.subtotal, 0) });
const deferred = () => { let resolve; const promise = new Promise(r => resolve = r); return { promise, resolve }; };
function setup(options = {}) {
  let page, reads = 0, saves = 0, downloads = 0, reset;
  let server = cart(options.items || [row('1'), row('2')]);
  const storage = {}, toasts = [], navigations = [], updates = [], removals = [], confirmations = [], modals = [];
  let confirmation = true, deleteFailure = false;
  const code = ts.transpileModule(fs.readFileSync('apps/wechat-mini/miniprogram/pages/cart/index.ts', 'utf8'), { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 } }).outputText;
  const modules = {
    guard: { protectedPage: o => page = o },
    presentation: { mediaUrl: path => !path ? '' : /^https?:/.test(path) ? path : 'https://api.example' + path },
    cart: {
      getCart: async () => { reads++; if (options.read) return options.read(reads, server); if (reads > 1 && options.refreshFailure) throw Error('refresh failed'); return server; },
      saveDemand: async () => { saves++; throw Error('must not save cart notes'); },
      updateQuantity: async (id, quantity) => { updates.push([id, quantity]); if (options.updateFailure) throw Error('update failed'); server = cart(server.items.map(i => i.id === id ? { ...i, quantity, subtotal: i.unitPrice * quantity } : i)); return server; },
      removeItem: async id => { removals.push(id); if (deleteFailure) throw Error('delete failed'); server = cart(server.items.filter(i => i.id !== id)); return server; },
      confirmCartItem: async (id, version) => { confirmations.push([id, version]); server = cart(server.items.map(i => i.id === id ? { ...i, needsConfirmation: false, unitPrice: 250, subtotal: 250 * i.quantity } : i)); return server; },
    },
    session: { onSessionReset: fn => reset = fn },
    request: { download: async () => { downloads++; throw Error('must not download hidden media'); } },
  };
  vm.runInNewContext(code, { exports: {}, Error, encodeURIComponent, require: p => modules[p.split('/').at(-1)], wx: {
    getWindowInfo: () => ({ windowWidth: 375 }), showToast: o => toasts.push(o.title),
    setStorageSync: (k, v) => { if (options.storageFailure) throw Error('storage failed'); storage[k] = v; }, removeStorageSync: k => delete storage[k],
    navigateTo: async o => { navigations.push(o.url); if (options.navigationFailure) throw Error('navigation failed'); },
    showModal: async o => { modals.push(o); return { confirm: confirmation }; },
    showActionSheet: async o => { assert.deepEqual(Array.from(o.itemList), ['删除服务']); return { tapIndex: 0 }; },
  }, console });
  page.setData = function (update) { for (const [k, v] of Object.entries(update)) { if (k === 'cart.items') this.data.cart.items = v; else this.data[k] = v; } };
  page.onLoad();
  return { page, storage, toasts, navigations, updates, removals, confirmations, modals, reset: () => reset(), counts: () => ({ reads, saves, downloads }), setServer: items => server = cart(items), cancel: () => confirmation = false, failDelete: () => deleteFailure = true };
}
test('selection, distinct item count, real totals and selected-only checkout; no demand writes/downloads', async () => {
  const s = setup({ items: [row('1', { quantity: 2, subtotal: 200 }), row('2', { quantity: 3, subtotal: 300 }), row('3', { available: false })] });
  await s.page.load(); assert.equal(s.page.data.selectedCount, 2); assert.equal(s.page.data.totalText, '5.00');
  s.page.toggleAll(); assert.equal(s.page.data.selectedCount, 0); assert.equal(s.page.data.totalText, '0.00');
  s.page.toggleAll(); s.page.toggleSelected(event('2')); await s.page.checkout();
  assert.deepEqual(Array.from(s.storage[key]), ['1']); assert.equal(s.navigations.length, 1); assert.equal(s.page.data.checkoutLoading, false);
  assert.equal(s.counts().saves, 0); assert.equal(s.counts().downloads, 0);
  assert.equal(s.page.data.cart.items[0].demandDescription, '上门前联系'); assert.equal(s.page.data.cart.items[0].demandMedia[0].id, 'media-1');
});
for (const kind of ['refreshFailure', 'navigationFailure', 'storageFailure']) test(`${kind} releases checkout lock`, async () => {
  const s = setup({ [kind]: true }); await s.page.load(); await s.page.checkout();
  assert.equal(s.page.data.checkoutLoading, false); s.page.toggleAll(); assert.equal(s.page.data.selectedCount, 0);
  s.page.toggleAll(); assert.equal(s.page.data.selectedCount, 2); assert.ok(s.toasts.length); assert.equal(s.storage[key], undefined);
});
test('unexpected synchronous apply failure releases checkout lock', async () => {
  const s = setup(); await s.page.load(); s.page.applyCart = () => { throw Error('unexpected'); }; await s.page.checkout(); assert.equal(s.page.data.checkoutLoading, false);
});
test('concurrent checkout taps navigate once; selection locked while verifying', async () => {
  const d = deferred(); const s = setup({ read: (n, server) => n === 1 ? server : d.promise }); await s.page.load();
  const pending = s.page.checkout(); s.page.toggleAll(); await s.page.checkout(); assert.equal(s.page.data.selectedCount, 2);
  d.resolve(cart([row('1'), row('2')])); await pending; assert.equal(s.navigations.length, 1);
});
for (const action of ['change', 'remove']) test(`old refresh cannot overwrite ${action} response`, async () => {
  const d = deferred(); const s = setup({ read: (n, server) => n === 2 ? d.promise : server }); await s.page.load();
  const pending = s.page.load(); s.page.toggleSelected(event('2')); await s.page[action](event('1', 1));
  d.resolve(cart([row('1'), row('2')])); await pending;
  assert.equal(s.page.data.cart.items.some(i => i.id === '1'), action !== 'remove');
  if (action === 'change') assert.equal(s.page.data.cart.items[0].quantity, 2);
  assert.equal(s.page.data.cart.items.find(i => i.id === '2').selected, false); assert.equal(s.page.data.loading, false);
});
test('newer refresh wins; unload and session reset ignore old private responses', async () => {
  for (const boundary of ['newer', 'unload', 'session', 'hide']) {
    const d = deferred(); const s = setup({ read: (n, server) => n === 2 ? d.promise : server }); await s.page.load(); const pending = s.page.load();
    if (boundary === 'newer') { s.setServer([row('new')]); await s.page.load(); }
    if (boundary === 'unload') s.page.onUnload();
    if (boundary === 'session') s.reset();
    if (boundary === 'hide') s.page.onHide();
    d.resolve(cart([row('stale')])); await pending;
    assert.equal(s.page.data.cart.items.some(i => i.id === 'stale'), false);
    if (boundary === 'session') assert.equal(s.page.data.cart.items.length, 0);
  }
});
test('stable ID updates after reorder, selection survives quantity/detail round trip and bounds', async () => {
  const s = setup({ items: [row('1'), row('2', { quantity: 9999, subtotal: 999900 })] }); await s.page.load(); s.page.toggleSelected(event('2'));
  s.setServer([row('2', { quantity: 9999, subtotal: 999900 }), row('1')]); await s.page.load();
  await s.page.change(event('1', -1)); await s.page.change(event('2', 1)); await s.page.change(event('1', 0.5)); assert.equal(s.updates.length, 0);
  await s.page.change(event('1', 1)); assert.deepEqual(s.updates, [['1', 2]]); assert.equal(s.page.data.totalText, '2.00');
  await s.page.openDetail(event('1')); assert.equal(s.navigations.at(-1), '/pages/services/detail?id=sku-1');
  s.page.onHide(); s.page.onShow(); await s.page.load(); assert.equal(s.page.data.cart.items.find(i => i.id === '2').selected, false);
});
test('update failure and deletion cancellation/failure keep data and recover controls', async () => {
  const s = setup({ updateFailure: true }); await s.page.load(); await s.page.change(event('1', 1)); assert.equal(s.page.data.updatingId, ''); assert.equal(s.page.data.cart.items[0].quantity, 1);
  s.cancel(); await s.page.remove(event('1')); assert.equal(s.removals.length, 0); assert.equal(s.page.data.cart.items.length, 2);
  const f = setup(); await f.page.load(); f.failDelete(); await f.page.remove(event('1')); assert.equal(f.page.data.cart.items.length, 2); assert.equal(f.page.data.updatingId, '');
});
test('last unavailable item remains removable and long press confirms only once, suppressing tail click', async () => {
  const s = setup({ items: [row('1', { available: false })] }); await s.page.load(); s.page.toggleSelected(event('1')); await s.page.change(event('1', 1));
  assert.equal(s.page.data.selectedCount, 0); assert.equal(s.updates.length, 0); await s.page.longPress(event('1')); await s.page.openDetail(event('1'));
  assert.equal(s.modals.length, 1); assert.equal(s.removals.length, 1); assert.equal(s.page.data.cart.items.length, 0); assert.equal(s.navigations.length, 0);
});
test('swipe and vertical scroll suppress tail navigation; quantity/selection controls are excluded from row gestures', async () => {
  const s = setup(); await s.page.load();
  const touch = (x, y) => ({ ...event('1'), touches: [{ clientX: x, clientY: y }] });
  s.page.touchStart(touch(200, 100)); s.page.touchMove(touch(125, 102)); await s.page.longPress(event('1')); assert.equal(s.modals.length, 0);
  s.page.touchEnd(); assert.equal(s.page.data.swipeOffset, 72); await s.page.openDetail(event('1')); assert.equal(s.navigations.length, 0);
  s.page.touchStart(touch(125, 100)); s.page.touchMove(touch(200, 102)); s.page.touchEnd(); assert.equal(s.page.data.swipeId, '');
  s.page.touchStart(touch(200, 100)); s.page.touchMove(touch(198, 180)); s.page.touchEnd(); await s.page.openDetail(event('1')); assert.equal(s.navigations.length, 0);
  s.page.touchStart({...touch(200, 100),target:{dataset:{control:true}}}); s.page.touchMove(touch(100,100)); s.page.touchEnd(); await s.page.longPress({...event('1'),target:{dataset:{control:true}}}); assert.equal(s.page.data.swipeId,''); assert.equal(s.modals.length,0);
});
for (const reason of ['removed', 'unavailable', 'version']) test(`${reason} during checkout blocks navigation; explicit version confirmation uses server result`, async () => {
  const s = setup(); await s.page.load(); s.page.toggleSelected(event('2'));
  s.setServer(reason === 'removed' ? [row('2')] : [row('1', reason === 'version' ? { needsConfirmation: true } : { available: false }), row('2')]);
  await s.page.checkout(); assert.equal(s.navigations.length, 0); assert.equal(s.page.data.checkoutLoading, false); assert.ok(s.toasts.length);
  if (reason === 'version') { assert.equal(s.page.data.totalText, '1.00'); await s.page.confirmVersion(event('1')); assert.deepEqual(s.confirmations, [['1', 2]]); assert.equal(s.page.data.totalText, '2.50'); await s.page.checkout(); assert.deepEqual(Array.from(s.storage[key]), ['1']); }
});
test('image handling preserves absolute URLs and fallback does not affect selection', async () => {
  const s = setup({ items: [row('1', { coverImageUrl: 'https://cdn.example/a.jpg' }), row('2', { coverImageUrl: '/cover' })] }); await s.page.load();
  assert.equal(s.page.data.cart.items[0].coverImageUrl, 'https://cdn.example/a.jpg'); assert.equal(s.page.data.cart.items[1].coverImageUrl, 'https://api.example/cover');
  s.page.imageError(event('1')); assert.equal(s.page.data.cart.items[0].coverImageUrl, ''); assert.equal(s.page.data.selectedCount, 2);
});
