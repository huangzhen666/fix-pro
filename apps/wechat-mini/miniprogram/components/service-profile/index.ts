import { getApiBaseUrl } from '../../config/env'
import type { ServiceProfile } from '../../services/serviceProfile'
const url = (id?: string) => id ? `${getApiBaseUrl()}/api/v1/public/media/${id}` : ''
const rows = <T extends object>(items?: T[]) => items?.map((item, index) => ({ ...item, viewKey: index }))
Component({
  properties: { profile: { type: Object, value: {} } },
  data: { openFaq: -1, view: null as any },
  observers: {
    profile(value: ServiceProfile | null) {
      if (!value) { this.setData({ view: null }); return }
      const h = value.enhancements, a = value.afterSales
      this.setData({ view: {
        ...value,
        items: (value.includedItems || []).map((v, index) => ({ ...v, viewKey: index, imageUrl: url(v.imageMediaId), iconUrl: `/assets/icons/service-${v.iconKey || 'service'}.svg` })),
        exclusions: rows(value.excludedItems),
        h: h ? { ...h,
          facts: rows(h.facts), steps: rows(h.steps), faqs: rows(h.faqs),
          resources: h.resources?.map((v, index) => ({ ...v, viewKey: index, providerText: { PROVIDER: '服务方提供', CUSTOMER: '客户准备', SHARED: '双方准备' }[v.provider] })),
          gallery: h.gallery?.map((v, index) => ({ ...v, viewKey: index, url: url(v.mediaId) })),
          comparisons: h.comparisons?.map((v, index) => ({ ...v, viewKey: index, beforeUrl: url(v.beforeMediaId), afterUrl: url(v.afterMediaId) })),
        } : null,
        policyTitle: a ? { WARRANTY: '质保说明', REWORK: '复洁或重做说明', SUPPORT: '售后支持' }[a.type] : '售后支持',
        periodText: a?.period ? `${a.period.value}${{ HOUR: '小时', DAY: '天', MONTH: '个月' }[a.period.unit]} · ${a.period.startDescription}` : '',
      } })
    },
  },
  methods: {
    toggleFaq(e: any) { const i = Number(e.currentTarget.dataset.index); this.setData({ openFaq: this.data.openFaq === i ? -1 : i }) },
    imageError(e: any) { const path = e.currentTarget.dataset.path; if (path) this.setData({ [path]: '/assets/icons/service-service.svg' }) },
    preview(e: any) { const src = e.currentTarget.dataset.src; if (src) wx.previewImage({ urls: [src] }) },
  },
})
