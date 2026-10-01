const warningWindowMs = 30 * 60 * 1000

function appointmentEndAt(appointmentAt?: string, appointmentSlot?: string) {
  if (!appointmentAt || !/^(08|10|12|14|16|18|20):00$/.test(appointmentSlot ?? '')) return undefined
  const date = new Date(appointmentAt)
  if (Number.isNaN(date.getTime())) return undefined
  const hour = Number(appointmentSlot!.slice(0, 2))
  return new Date(date.getFullYear(), date.getMonth(), date.getDate(), hour + 2, 0, 0, 0)
}

export function acceptanceReminder(status: string, appointmentAt?: string, appointmentSlot?: string) {
  if (status !== 'PENDING_ACCEPT') return { expired: false, message: '' }
  const deadline = appointmentEndAt(appointmentAt, appointmentSlot)
  if (!deadline) return { expired: false, message: '' }
  const remaining = deadline.getTime() - Date.now()
  if (remaining <= 0) return { expired: true, message: '预约时间段已结束，不能再接单，请联系调度员。' }
  if (remaining <= warningWindowMs) return { expired: false, message: `预约时间段即将结束，请在 ${Math.ceil(remaining / 60000)} 分钟内接单。` }
  return { expired: false, message: '' }
}
