import { getApiBaseUrl } from '../config/env'

interface ApiResponse<T> {
  code: string
  message: string
  data: T
}

export class ApiError extends Error {
  constructor(public readonly code: string, message: string) {
    super(message)
  }
}

export function setWorkerToken(token: string): void {
  wx.setStorageSync('fixpro.worker.accessToken', token)
}

export function request<T>(options: WechatMiniprogram.RequestOption): Promise<T> {
  const token = wx.getStorageSync<string>('fixpro.worker.accessToken')
  return new Promise((resolve, reject) => {
    wx.request<ApiResponse<T>>({
      ...options,
      url: `${getApiBaseUrl()}${options.url}`,
      header: {
        Accept: 'application/json',
        'Content-Type': 'application/json',
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
        ...(options.header ?? {}),
      },
      success(response) {
        if (response.statusCode >= 200 && response.statusCode < 300 && response.data.code === 'OK') {
          resolve(response.data.data)
          return
        }
        reject(new ApiError(response.data?.code ?? `HTTP_${response.statusCode}`, response.data?.message ?? '请求失败'))
      },
      fail(error) {
        reject(new Error(`请求失败：${error.errMsg || '网络不可用'}`))
      },
    })
  })
}

export function upload<T>(path: string, filePath: string): Promise<T> {
  const token = wx.getStorageSync<string>('fixpro.worker.accessToken')
  return new Promise((resolve, reject) => {
    wx.uploadFile({
      url: `${getApiBaseUrl()}${path}`,
      filePath,
      name: 'file',
      header: token ? { Authorization: `Bearer ${token}` } : {},
      success(response) {
        try {
          const payload = JSON.parse(response.data) as ApiResponse<T>
          if (response.statusCode >= 200 && response.statusCode < 300 && payload.code === 'OK') resolve(payload.data)
          else reject(new ApiError(payload.code, payload.message))
        } catch {
          reject(new Error('上传响应解析失败'))
        }
      },
      fail: reject,
    })
  })
}
