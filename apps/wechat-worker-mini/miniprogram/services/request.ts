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

export function setWorkerProfile(profile: unknown): void {
  wx.setStorageSync('fixpro.worker.profile', profile)
}

export function clearWorkerSession(): void {
  wx.removeStorageSync('fixpro.worker.accessToken')
  wx.removeStorageSync('fixpro.worker.profile')
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
        const code = response.data?.code ?? `HTTP_${response.statusCode}`
        if (response.statusCode === 401) {
          clearWorkerSession()
          // 登录接口的 401 需要交给登录页展示“手机号或密码错误”，
          // 不能在请求层立刻重载当前页面，否则页面 catch 无法更新提示文案。
          if (options.url !== '/api/v1/worker/auth/login') {
            wx.reLaunch({ url: '/pages/login/index' })
          }
        } else if (response.statusCode === 423) {
          wx.reLaunch({ url: '/pages/change-password/index' })
        }
        reject(new ApiError(code, response.data?.message ?? '请求失败'))
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
          else {
            if (response.statusCode === 401) {
              clearWorkerSession()
              wx.reLaunch({ url: '/pages/login/index' })
            } else if (response.statusCode === 423) {
              wx.reLaunch({ url: '/pages/change-password/index' })
            }
            reject(new ApiError(payload.code, payload.message))
          }
        } catch {
          reject(new Error('上传响应解析失败'))
        }
      },
      fail: reject,
    })
  })
}
