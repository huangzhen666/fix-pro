import { ensureReady, hasSession, openLogin } from './session'
export const protectedPage: typeof Page = ((options: any) => {
  const load = options.onLoad; const show = options.onShow; const hide = options.onHide
  options.data = { ...options.data, sessionReady: false }
  options.onHide = function(this: any, ...args: any[]) { this.setData({ sessionReady: false }); return hide?.apply(this, args) }
  const ready = async function(this: any) {
    await ensureReady()
    if (!this.__customerLoaded) { this.__customerLoaded = true; try { await load?.apply(this, this.__customerArgs ?? []) } catch (e) { this.__customerLoaded = false; throw e } }
  }
  const failed = (e: unknown) => { if (!hasSession()) openLogin(); else wx.showToast({ title: e instanceof Error ? e.message : '登录校验失败', icon: 'none' }) }
  options.onLoad = function(this: any, ...args: any[]) {
    this.__customerArgs = args
    this.__customerLoad = ready.call(this).catch(failed)
  }
  options.onShow = async function(this: any, ...args: any[]) {
    await this.__customerLoad
    try { await ready.call(this); this.setData({ sessionReady: true }); return show?.apply(this, args) } catch (e) { failed(e) }
  }
  Page(options)
}) as typeof Page
