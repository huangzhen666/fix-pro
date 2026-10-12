// Native phone/location requests stay pending until the privacy agreement is accepted.
// The bundled SDK types describe this callback incorrectly; use the documented resolve signature.
export type PrivacyResolve = (result: { event: 'agree' | 'disagree'; buttonId?: string }) => void
const pendingPrivacy = new WeakMap<object, Set<PrivacyResolve>>()
let privacyListenerRegistered = false
export function registerPrivacyListener() {
  if (privacyListenerRegistered || !wx.onNeedPrivacyAuthorization) return
  const listen = wx.onNeedPrivacyAuthorization as unknown as (listener: (resolve: PrivacyResolve) => void) => void
  listen(resolve => {
    const pages = getCurrentPages()
    const page = pages[pages.length - 1] as unknown as { requestPrivacy?: (resolve: PrivacyResolve) => void } | undefined
    if (page?.requestPrivacy) page.requestPrivacy(resolve)
    else resolve({ event: 'disagree' })
  })
  privacyListenerRegistered = true
}
export function finishPrivacy(owner: object, accepted: boolean, buttonId = 'privacy-agree') {
  const requests = pendingPrivacy.get(owner)
  pendingPrivacy.delete(owner)
  requests?.forEach(resolve => resolve(accepted
    ? { event: 'agree', buttonId }
    : { event: 'disagree' }))
}

export function queuePrivacy(owner: object, resolve: PrivacyResolve) {
  const requests = pendingPrivacy.get(owner) || new Set<PrivacyResolve>()
  requests.add(resolve)
  pendingPrivacy.set(owner, requests)
}
