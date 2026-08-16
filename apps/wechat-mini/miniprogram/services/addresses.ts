import { request } from './request'

export interface CustomerAddress {
  id: string
  city: string
  detailAddress: string
  buildingDoor: string
  contactName: string
  contactMobile: string
  isDefault: boolean
  createdAt: string
  updatedAt: string
}

export type AddressWrite = Omit<CustomerAddress, 'id' | 'createdAt' | 'updatedAt'>

export function listAddresses() {
  return request<CustomerAddress[]>({ url: '/api/v1/mini/addresses', method: 'GET' })
}

export function createAddress(data: AddressWrite) {
  return request<CustomerAddress>({ url: '/api/v1/mini/addresses', method: 'POST', data })
}

export function updateAddress(id: string, data: AddressWrite) {
  return request<CustomerAddress>({ url: `/api/v1/mini/addresses/${id}`, method: 'PUT', data })
}

export function setDefaultAddress(id: string) {
  return request<{ ok: boolean }>({ url: `/api/v1/mini/addresses/${id}/default`, method: 'POST' })
}

export function deleteAddress(id: string) {
  return request<{ ok: boolean }>({ url: `/api/v1/mini/addresses/${id}`, method: 'DELETE' })
}

export function formatAddress(address: CustomerAddress) {
  return `${address.city}${address.detailAddress}${address.buildingDoor}`
}
