import { onSessionReset } from '../../services/session'
import { showApiError, request, ApiError } from '../../services/request'
import { protectedPage } from '../../services/guard'
import { getService } from '../../services/catalog'
import { getCart, saveDemand, type Cart } from '../../services/cart'
import { createOrder } from '../../services/orders'
import { formatAddress, listAddresses, type CustomerAddress } from '../../services/addresses'
import { mediaUrl } from '../../services/presentation'
import { demandFilled, type DemandDraftItem } from '../../services/demand'

const checkoutItemsStorageKey = 'fixpro.checkoutCartItemIds'
type AppointmentDateOption = { value: string; label: string; selected: boolean }
type AppointmentSlotOption = { value: string; label: string; period: string; showPeriod: boolean; selected: boolean }

const slotDefinitions: Array<Pick<AppointmentSlotOption, 'value' | 'label' | 'period'>> = [
  { value: '08:00', label: '08:00–10:00', period: '上午' },
  { value: '10:00', label: '10:00–12:00', period: '上午' },
  { value: '12:00', label: '12:00–14:00', period: '下午' },
  { value: '14:00', label: '14:00–16:00', period: '下午' },
  { value: '16:00', label: '16:00–18:00', period: '下午' },
]

const weekNames = ['日', '一', '二', '三', '四', '五', '六']

function selectedCart(cart: Cart, itemIDs: string[] | null): Cart {
  if (!itemIDs) return cart
  const ids = new Set(itemIDs)
  const items = cart.items.filter(item => ids.has(item.id))
  return { ...cart, items, itemCount: items.reduce((total, item) => total + item.quantity, 0), totalAmount: items.reduce((total, item) => total + item.subtotal, 0) }
}

function pad(value: number) {
  return String(value).padStart(2, '0')
}

function dateValue(date: Date) {
  return `${date.getUTCFullYear()}-${pad(date.getUTCMonth() + 1)}-${pad(date.getUTCDate())}`
}

function dateLabel(date: Date, offset: number) {
  const suffix = offset === 1 ? '明天' : `周${weekNames[date.getUTCDay()]}`
  return `${pad(date.getUTCMonth() + 1)}月${pad(date.getUTCDate())}日（${suffix}）`
}

function buildAppointmentDates(): AppointmentDateOption[] {
  const today = new Date(Date.now() + 8 * 60 * 60 * 1000)
  return Array.from({ length: 30 }, (_, index) => {
    const offset = index + 1
    const date = new Date(Date.UTC(today.getUTCFullYear(), today.getUTCMonth(), today.getUTCDate() + offset))
    return { value: dateValue(date), label: dateLabel(date, offset), selected: offset === 1 }
  })
}

function buildAppointmentSlots(selected = ''): AppointmentSlotOption[] {
  return slotDefinitions.map((slot, index) => ({ ...slot, showPeriod: index === 0 || slot.period !== slotDefinitions[index - 1].period, selected: slot.value === selected }))
}

type CheckoutViewItem = Cart['items'][number] & { priceText: string; subtotalText: string; imageFailed: boolean }
const sources = new WeakMap<object, { options: Record<string, string>; ids: string[] | null }>()
const attempts = new WeakMap<object, { fingerprint: string; key: string; prepared: boolean }>()
const loads = new WeakMap<object, number>()

protectedPage({
  data: {
    cart: { items: [], itemCount: 0, totalAmount: 0 } as Cart,
    services: [] as CheckoutViewItem[], totalText: '0.00', loading: true, loadError: '',
    directItem: null as { skuId: string; skuVersion: number; quantity: number } | null,
    contactName: '', contactMobile: '', serviceAddress: '', selectedAddress: null as CustomerAddress | null,
    appointmentDate: '', appointmentDateLabel: '', appointmentSlot: '', appointmentSlotLabel: '', appointmentDisplayText: '请选择期望上门时间',
    pickerDate: '', pickerSlot: '', appointmentDates: [] as AppointmentDateOption[], appointmentSlots: buildAppointmentSlots(),
    showAppointmentPicker: false, submitting: false, uploading: false, submissionUnknown: false,
    addressError: '', appointmentError: '', showDemandEditor: false, demandDraft: [] as DemandDraftItem[], editorItems: [] as DemandDraftItem[], hasDemand: false,
  },
  async onLoad(options: Record<string, string>) {
    const value = wx.getStorageSync<unknown>(checkoutItemsStorageKey)
    wx.removeStorageSync(checkoutItemsStorageKey)
    sources.set(this, { options, ids: Array.isArray(value) ? value.filter((id): id is string => typeof id === 'string') : null })
    await this.retryLoad()
  },
  async retryLoad() {
    const source = sources.get(this)
    if (!source) return
    const epoch = (loads.get(this) || 0) + 1; loads.set(this, epoch)
    this.setData({ loading: true, loadError: '' })
    try {
      let directItem: typeof this.data.directItem = null, cart: Cart
      if (source.options.skuId) {
        const quantity = Number(source.options.quantity), skuVersion = Number(source.options.skuVersion)
        if (!Number.isInteger(quantity) || quantity < 1 || quantity > 9999 || !Number.isInteger(skuVersion) || skuVersion < 1) throw new Error('下单参数无效，请返回服务详情重新选择')
        const service = await getService(source.options.skuId)
        if (service.publishedVersion !== skuVersion) throw new Error('服务信息已更新，请返回详情确认后重新下单')
        directItem = { skuId: service.id, skuVersion, quantity }
        cart = { items: [{ id: 'direct', skuId: service.id, skuVersion, name: service.name, coverImageUrl: service.coverImageUrl, unitPrice: service.price, unit: service.unit, quantity, subtotal: service.price * quantity, serviceDescription: service.serviceDescription, serviceNotice: service.serviceNotice, demandMedia: [], demandComplete: false, available: true, needsConfirmation: false, currentVersion: skuVersion }], itemCount: quantity, totalAmount: service.price * quantity }
      } else cart = selectedCart(await getCart(), source.ids)
      if (!cart.items.length) throw new Error('没有可结算的服务，请返回重新选择')
      if (source.ids && cart.items.length !== new Set(source.ids).size) throw new Error('所选服务已变化，请返回购物车重新选择')
      if (cart.items.some(item => !item.available || item.needsConfirmation)) throw new Error('服务已更新或下架，请返回核对后重新下单')
      const [addresses, contact] = await Promise.all([listAddresses(), request<{ contactMobile: string }>({ url: '/api/v1/mini/checkout/contact' })])
      if (loads.get(this) !== epoch) return
      const selectedAddress = this.data.selectedAddress || addresses.find(item => item.isDefault) || addresses[0] || null
      const demandDraft = cart.items.map(item => this.data.demandDraft.find(draft => draft.itemId === item.id) || { itemId: item.id, name: item.name, demandDescription: item.demandDescription || '', demandMedia: item.demandMedia })
      this.setData({ cart, directItem, services: cart.items.map(item => ({ ...item, coverImageUrl: mediaUrl(item.coverImageUrl), priceText: (item.unitPrice / 100).toFixed(2), subtotalText: (item.subtotal / 100).toFixed(2), imageFailed: !item.coverImageUrl })), totalText: (cart.totalAmount / 100).toFixed(2), appointmentDates: buildAppointmentDates(), selectedAddress, contactName: selectedAddress?.contactName || '', contactMobile: selectedAddress?.contactMobile || contact.contactMobile, serviceAddress: selectedAddress ? formatAddress(selectedAddress) : '', demandDraft, editorItems: demandDraft, hasDemand: demandFilled(demandDraft) })
    } catch (e) { if (loads.get(this) === epoch) this.setData({ loadError: e instanceof Error ? e.message : '服务加载失败，请重试' }) }
    finally { if (loads.get(this) === epoch) this.setData({ loading: false }) }
  },
  onUnload() { loads.delete(this); sources.delete(this); attempts.delete(this) },
  onShow() {
    const selected = wx.getStorageSync<CustomerAddress | null>('fixpro.selectedAddress')
    if (!selected || this.data.submissionUnknown) return
    wx.removeStorageSync('fixpro.selectedAddress')
    this.setData({ selectedAddress: selected, contactName: selected.contactName, contactMobile: selected.contactMobile, serviceAddress: formatAddress(selected), addressError: '' })
  },
  back() { wx.navigateBack() },
  coverError(e: WechatMiniprogram.BaseEvent) { const index = this.data.services.findIndex(item => item.id === String(e.currentTarget.dataset.id)); if (index >= 0) this.setData({ [`services[${index}].imageFailed`]: true }) },
  openAddressPicker() { if (!this.data.submitting && !this.data.submissionUnknown) wx.navigateTo({ url: '/pages/addresses/index?select=1' }) },
  openAppointmentPicker() {
    if (this.data.submitting || this.data.submissionUnknown) return
    const pickerDate = this.data.appointmentDate || this.data.appointmentDates[0]?.value || ''
    this.setData({ showAppointmentPicker: true, pickerDate, pickerSlot: this.data.appointmentSlot, appointmentDates: this.data.appointmentDates.map(item => ({ ...item, selected: item.value === pickerDate })), appointmentSlots: buildAppointmentSlots(this.data.appointmentSlot) })
  },
  cancelAppointmentPicker() { this.setData({ showAppointmentPicker: false }) },
  chooseAppointmentDate(e: WechatMiniprogram.BaseEvent) { const pickerDate = String(e.currentTarget.dataset.value); this.setData({ pickerDate, appointmentDates: this.data.appointmentDates.map(item => ({ ...item, selected: item.value === pickerDate })) }) },
  chooseAppointmentSlot(e: WechatMiniprogram.BaseEvent) { const pickerSlot = String(e.currentTarget.dataset.value); this.setData({ pickerSlot, appointmentSlots: buildAppointmentSlots(pickerSlot) }) },
  confirmAppointmentPicker() {
    const date = this.data.appointmentDates.find(item => item.value === this.data.pickerDate), slot = this.data.appointmentSlots.find(item => item.value === this.data.pickerSlot)
    if (!date || !slot) { wx.showToast({ title: '请选择预约日期和时间段', icon: 'none' }); return }
    this.setData({ showAppointmentPicker: false, appointmentDate: date.value, appointmentDateLabel: date.label, appointmentSlot: slot.value, appointmentSlotLabel: slot.label, appointmentDisplayText: `${date.label} ${slot.label}`, appointmentError: '' })
  },
  openDemand() { if (!this.data.submitting && !this.data.submissionUnknown) this.setData({ editorItems: this.data.demandDraft, showDemandEditor: true }) },
  demandChange(e: WechatMiniprogram.CustomEvent<{ items: DemandDraftItem[] }>) { this.setData({ demandDraft: e.detail.items, hasDemand: demandFilled(e.detail.items) }) },
  demandBusy(e: WechatMiniprogram.CustomEvent<{ busy: boolean }>) { this.setData({ uploading: e.detail.busy }) },
  closeDemand() { this.setData({ showDemandEditor: false }) },
  saveDemandDraft(e: WechatMiniprogram.CustomEvent<{ items: DemandDraftItem[] }>) { this.demandChange(e); this.closeDemand() },
  async submit() {
    if (this.data.loading || this.data.loadError || !this.data.cart.items.length || this.data.submitting || this.data.uploading) return
    const { contactName, contactMobile, serviceAddress, appointmentDate, appointmentSlot } = this.data
    const addressError = !this.data.selectedAddress || contactName.trim().length < 2 || !/^1\d{10}$/.test(contactMobile) || serviceAddress.trim().length < 5 ? '请添加或完善服务地址及联系人' : ''
    const appointmentError = !appointmentDate || !slotDefinitions.some(slot => slot.value === appointmentSlot) ? '请选择预约上门时间' : ''
    this.setData({ addressError, appointmentError })
    if (addressError || appointmentError) { wx.pageScrollTo({ selector: addressError ? '#service-address' : '#appointment', duration: 250 }); return }
    this.setData({ submitting: true })
    const first = this.data.demandDraft[0]
    const payload = { contactName, contactMobile, serviceAddress, appointmentDate, appointmentSlot, ...(this.data.directItem ? { directItem: { ...this.data.directItem, demandDescription: first.demandDescription, mediaIds: first.demandMedia.map(media => media.id) } } : { cartItemIds: this.data.cart.items.map(item => item.id) }) }
    const fingerprint = JSON.stringify({ payload, demand: this.data.demandDraft.map(item => ({ id: item.itemId, text: item.demandDescription, media: item.demandMedia.map(media => media.id) })) })
    let attempt = attempts.get(this)
    if (!attempt || attempt.fingerprint !== fingerprint) { attempt = { fingerprint, key: `${Date.now()}-${Math.random().toString(16).slice(2)}`, prepared: false }; attempts.set(this, attempt) }
    let creating = false
    try {
      if (!attempt.prepared && !this.data.directItem) for (const item of this.data.demandDraft) await saveDemand(item.itemId, item.demandDescription, item.demandMedia.map(media => media.id))
      attempt.prepared = true; creating = true
      const result = await createOrder(payload, attempt.key)
      this.setData({ submissionUnknown: false }); wx.redirectTo({ url: `/pages/orders/detail?id=${result.id}&pay=1` })
    } catch (e) {
      const unknown = creating && (!(e instanceof ApiError) || !e.status || e.status >= 500)
      this.setData({ submissionUnknown: unknown })
      if (!unknown) attempts.delete(this)
      if (e instanceof ApiError && ['SKU_NOT_AVAILABLE', 'CART_SKU_CHANGED'].includes(e.code)) wx.showModal({ title: '服务信息已变化', content: '请返回核对服务及价格后重新下单。', showCancel: false, success: () => wx.navigateBack() })
      else if (unknown) wx.showToast({ title: '提交结果待确认，请点击重试', icon: 'none' })
      else showApiError(e)
    } finally { this.setData({ submitting: false }) }
  },
})

onSessionReset(() => {
  for (const page of getCurrentPages()) {
    if (page.route !== 'pages/checkout/index') continue
    loads.delete(page); attempts.delete(page)
    page.setData({ cart: { items: [], itemCount: 0, totalAmount: 0 }, services: [], demandDraft: [], editorItems: [], showDemandEditor: false, showAppointmentPicker: false, selectedAddress: null, contactName: '', contactMobile: '', serviceAddress: '', hasDemand: false, submitting: false, uploading: false, submissionUnknown: false, loading: false, loadError: '登录状态已变化，请重新加载订单' })
  }
})
