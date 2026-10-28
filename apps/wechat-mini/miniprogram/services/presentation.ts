import { getApiBaseUrl } from '../config/env'

const categoryIcons: Record<string, string> = {
  '1001': 'droplets', '1002': 'wrench', '1003': 'shield-check',
  '1004': 'heater', '1005': 'appliance', '1006': 'house',
}
const categoryNames: Array<[RegExp, string]> = [
  [/漏水|测漏/, 'droplets'], [/水路|水管/, 'wrench'], [/防水/, 'shield-check'],
  [/暖气|净水/, 'heater'], [/家电|清洗/, 'appliance'], [/家装|墙面/, 'house'],
]
export function categoryIcon(id: string, name: string): string {
  const icon = categoryIcons[id] || categoryNames.find(([pattern]) => pattern.test(name))?.[1] || 'grid'
  return `/assets/icons/${icon}.svg`
}
export function mediaUrl(path: string): string {
  if (!path) return ''
  if (/^https?:\/\//i.test(path)) return path
  return `${getApiBaseUrl()}${path.startsWith('/') ? '' : '/'}${path}`
}

export function categoryLabel(name: string): string {
  return name === '暖气净水维保' ? '暖气净水' : name
}
