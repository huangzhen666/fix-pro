import { getCart, type Cart } from '../../services/cart'
import { createOrder } from '../../services/orders'
import { formatAddress, listAddresses, type CustomerAddress } from '../../services/addresses'
import { ApiError } from '../../services/request'
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

Page({
  data: {
    cart: { items: [], itemCount: 0, totalAmount: 0 } as Cart,
    totalText: '0.00',
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
  async onLoad() {
    const [cart, addresses] = await Promise.all([getCart(), listAddresses()])
    if (!cart.items.length) {
      wx.showToast({ title: '购物车为空', icon: 'none' })
      setTimeout(() => wx.navigateBack(), 500)
      return
    }
    const appointmentDates = buildAppointmentDates()
    const selectedAddress = addresses.find(item => item.isDefault) || addresses[0] || null
    this.setData({ cart, totalText: (cart.totalAmount / 100).toFixed(2), appointmentDates, selectedAddress, contactName: selectedAddress?.contactName || '', contactMobile: selectedAddress?.contactMobile || '', serviceAddress: selectedAddress ? formatAddress(selectedAddress) : '' })
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
      const result = await createOrder({ contactName, contactMobile, serviceAddress, appointmentDate, appointmentSlot }, key)
      wx.redirectTo({ url: `/pages/checkout/result?orderNo=${result.orderNo}&status=${result.status}&amount=${result.totalAmount}` })
    } catch (e: any) {
      if (e instanceof ApiError && e.code === 'CART_SKU_CHANGED') {
        wx.showModal({
          title: '服务信息已更新',
          content: '服务价格或内容已变化，请返回购物车刷新并确认后再提交。',
          showCancel: false,
          success: () => wx.navigateBack(),
        })
      } else {
        wx.showToast({ title: e instanceof Error ? e.message : '下单失败', icon: 'none' })
      }
    } finally {
      this.setData({ submitting: false })
    }
  },
})
