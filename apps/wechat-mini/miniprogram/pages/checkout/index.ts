import { showApiError } from '../../services/request'
import { protectedPage } from '../../services/guard'
import { getService } from '../../services/catalog'
import { getCart, type Cart } from '../../services/cart'
import { createOrder } from '../../services/orders'
import { formatAddress, listAddresses, type CustomerAddress } from '../../services/addresses'
import { request, ApiError } from '../../services/request'

const checkoutItemsStorageKey = 'fixpro.checkoutCartItemIds'
type AppointmentDateOption = { value: string; label: string; selected: boolean }
type AppointmentSlotOption = { value: string; label: string; period: string; showPeriod: boolean; selected: boolean }

const slotDefinitions: Array<Pick<AppointmentSlotOption, 'value' | 'label' | 'period'>> = [
  { value: '08:00', label: '08:00–10:00', period: '上午' },
  { value: '10:00', label: '10:00–12:00', period: '上午' },
  { value: '12:00', label: '12:00–14:00', period: '下午' },
  { value: '14:00', label: '14:00–16:00', period: '下午' },
  { value: '16:00', label: '16:00–18:00', period: '下午' },
  { value: '18:00', label: '18:00–20:00', period: '晚上' },
  { value: '20:00', label: '20:00–22:00', period: '晚上' },
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
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`
}

function dateLabel(date: Date, offset: number) {
  const suffix = offset === 1 ? '明天' : `周${weekNames[date.getDay()]}`
  return `${pad(date.getMonth() + 1)}月${pad(date.getDate())}日（${suffix}）`
}

function buildAppointmentDates(): AppointmentDateOption[] {
  const today = new Date()
  return Array.from({ length: 30 }, (_, index) => {
    const offset = index + 1
    const date = new Date(today.getFullYear(), today.getMonth(), today.getDate() + offset)
    return { value: dateValue(date), label: dateLabel(date, offset), selected: offset === 1 }
  })
}

function buildAppointmentSlots(selected = ''): AppointmentSlotOption[] {
  return slotDefinitions.map((slot, index) => ({ ...slot, showPeriod: index === 0 || slot.period !== slotDefinitions[index - 1].period, selected: slot.value === selected }))
}

protectedPage({
  data: {
    cart: { items: [], itemCount: 0, totalAmount: 0 } as Cart,
    totalText: '0.00',
    directItem: null as { skuId: string; skuVersion: number; quantity: number } | null,
    contactName: '',
    contactMobile: '',
    serviceAddress: '',
    selectedAddress: null as CustomerAddress | null,
    appointmentDate: '',
    appointmentDateLabel: '',
    appointmentSlot: '',
    appointmentSlotLabel: '',
    appointmentDisplayText: '请选择日期和时间段',
    pickerDate: '',
    pickerSlot: '',
    appointmentDates: [] as AppointmentDateOption[],
    appointmentSlots: buildAppointmentSlots(),
    showAppointmentPicker: false,
    submitting: false,
  },
  async onLoad(options: Record<string, string>) {
    const storedItemIDs = wx.getStorageSync<unknown>(checkoutItemsStorageKey)
    wx.removeStorageSync(checkoutItemsStorageKey)
    const itemIDs = Array.isArray(storedItemIDs) ? storedItemIDs.filter((id): id is string => typeof id === 'string') : null
    let directItem: typeof this.data.directItem = null
    let cart: Cart
    if (options.skuId) {
      const quantity = Number(options.quantity)
      const skuVersion = Number(options.skuVersion)
      if (!Number.isInteger(quantity) || quantity < 1 || quantity > 99 || !Number.isInteger(skuVersion) || skuVersion < 1) throw new Error('下单参数无效，请返回服务详情重新选择')
      const service = await getService(options.skuId)
      if (service.publishedVersion !== skuVersion) throw new Error('服务信息已更新，请返回详情确认后重新下单')
      directItem = { skuId: service.id, skuVersion, quantity }
      cart = { items: [{ id: 'direct', skuId: service.id, skuVersion, name: service.name, coverImageUrl: service.coverImageUrl, unitPrice: service.price, unit: service.unit, quantity, subtotal: service.price * quantity, faultMedia: [], faultComplete: false, available: true }], itemCount: quantity, totalAmount: service.price * quantity }
    } else {
      cart = selectedCart(await getCart(), itemIDs)
    }
    if (cart.items.some(item => !item.available)) {
      wx.showModal({ title: '服务已下架', content: '所选服务已下架，无法下单，请返回购物车重新选择。', showCancel: false, success: () => wx.navigateBack() })
      return
    }
    const [addresses, contact] = await Promise.all([listAddresses(), request<{ contactMobile: string }>({ url: '/api/v1/mini/checkout/contact' })])
    if (!cart.items.length) {
      wx.showToast({ title: itemIDs ? '所选服务已不在购物车中' : '购物车为空', icon: 'none' })
      setTimeout(() => wx.navigateBack(), 500)
      return
    }
    const appointmentDates = buildAppointmentDates()
    const selectedAddress = addresses.find(item => item.isDefault) || addresses[0] || null
    this.setData({ cart, directItem, totalText: (cart.totalAmount / 100).toFixed(2), appointmentDates, selectedAddress, contactName: selectedAddress?.contactName || '', contactMobile: selectedAddress?.contactMobile || contact.contactMobile, serviceAddress: selectedAddress ? formatAddress(selectedAddress) : '' })
  },
  onShow() {
    const selected = wx.getStorageSync<CustomerAddress | null>('fixpro.selectedAddress')
    if (!selected) return
    wx.removeStorageSync('fixpro.selectedAddress')
    this.setData({ selectedAddress: selected, contactName: selected.contactName, contactMobile: selected.contactMobile, serviceAddress: formatAddress(selected) })
  },
  openAddressPicker() {
    wx.navigateTo({ url: '/pages/addresses/index?select=1' })
  },
  openAppointmentPicker() {
    const firstDate = this.data.appointmentDates[0]
    const pickerDate = this.data.appointmentDate || firstDate?.value || ''
    const appointmentDates = this.data.appointmentDates.map(item => ({ ...item, selected: item.value === pickerDate }))
    this.setData({ showAppointmentPicker: true, pickerDate, pickerSlot: this.data.appointmentSlot, appointmentDates, appointmentSlots: buildAppointmentSlots(this.data.appointmentSlot) })
  },
  cancelAppointmentPicker() {
    this.setData({ showAppointmentPicker: false })
  },
  chooseAppointmentDate(e: any) {
    const pickerDate = String(e.currentTarget.dataset.value)
    this.setData({ pickerDate, appointmentDates: this.data.appointmentDates.map(item => ({ ...item, selected: item.value === pickerDate })) })
  },
  chooseAppointmentSlot(e: any) {
    const pickerSlot = String(e.currentTarget.dataset.value)
    this.setData({ pickerSlot, appointmentSlots: buildAppointmentSlots(pickerSlot) })
  },
  confirmAppointmentPicker() {
    if (!this.data.pickerDate || !this.data.pickerSlot) {
      wx.showToast({ title: '请选择预约日期和时间段', icon: 'none' })
      return
    }
    const date = this.data.appointmentDates.find(item => item.value === this.data.pickerDate)
    const slot = this.data.appointmentSlots.find(item => item.value === this.data.pickerSlot)
    const appointmentDateLabel = date?.label || this.data.pickerDate
    const appointmentSlotLabel = slot?.label || this.data.pickerSlot
    this.setData({ showAppointmentPicker: false, appointmentDate: this.data.pickerDate, appointmentDateLabel, appointmentSlot: this.data.pickerSlot, appointmentSlotLabel, appointmentDisplayText: `${appointmentDateLabel} · ${appointmentSlotLabel}` })
  },
  async submit() {
    if (this.data.submitting) return
    const { contactName, contactMobile, serviceAddress, appointmentDate, appointmentSlot } = this.data
    if (contactName.trim().length < 2 || !/^1\d{10}$/.test(contactMobile) || serviceAddress.trim().length < 5 || !appointmentDate || !appointmentSlot) {
      wx.showToast({ title: '请完善联系人、地址和预约时间段', icon: 'none' })
      return
    }
    this.setData({ submitting: true })
    const key = `${Date.now()}-${Math.random().toString(16).slice(2)}`
    try {
      const result = await createOrder({ contactName, contactMobile, serviceAddress, appointmentDate, appointmentSlot, ...(this.data.directItem ? { directItem: this.data.directItem } : { cartItemIds: this.data.cart.items.map(item => item.id) }) }, key)
      wx.redirectTo({ url: `/pages/checkout/result?orderNo=${result.orderNo}&status=${result.status}&amount=${result.totalAmount}` })
    } catch (e: any) {
      if (e instanceof ApiError && e.code === 'SKU_NOT_AVAILABLE') {
        wx.showModal({ title: '服务已下架', content: this.data.directItem ? '服务已下架，无法下单，请返回重新选择。' : '所选服务已下架，无法下单，请返回购物车重新选择。', showCancel: false, success: () => wx.navigateBack() })
      } else if (e instanceof ApiError && e.code === 'CART_SKU_CHANGED') {
        wx.showModal({
          title: '服务信息已更新',
          content: this.data.directItem ? '服务价格或内容已变化，请返回详情确认后重新下单。' : '服务价格或内容已变化，请返回购物车刷新并确认后再提交。',
          showCancel: false,
          success: () => wx.navigateBack(),
        })
      } else {
        showApiError(e)
      }
    } finally {
      this.setData({ submitting: false })
    }
  },
})
