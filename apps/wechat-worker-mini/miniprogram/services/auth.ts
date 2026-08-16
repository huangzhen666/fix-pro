import { ApiError, request, setWorkerProfile, setWorkerToken, clearWorkerSession } from './request'

export interface WorkerProfile {
  id: number | string
  workerNo?: string
  displayName: string
  mobile?: string
  mobileMasked?: string
  avatar?: WorkerMedia
}

export interface WorkerMedia {
  id: string
  mediaType: string
  contentType?: string
  name?: string
  url: string
}

interface LoginResult {
  token: string
  expiresAt: string
  mustChangePassword: boolean
  worker: WorkerProfile
}

export interface MeResult {
  worker: WorkerProfile
  mustChangePassword: boolean
}

export function loginWorker(mobile: string, password: string): Promise<LoginResult> {
  return request<LoginResult>({ url: '/api/v1/worker/auth/login', method: 'POST', data: { mobile, password } }).then(result => {
    setWorkerToken(result.token)
    setWorkerProfile(result.worker)
    return result
  })
}

export function getWorkerMe(): Promise<MeResult> {
  return request<MeResult>({ url: '/api/v1/worker/auth/me' }).then(result => {
    setWorkerProfile(result.worker)
    return result
  })
}

export function changeWorkerPassword(currentPassword: string, newPassword: string, confirmPassword: string): Promise<{ changed: boolean }> {
  return request<{ changed: boolean }>({ url: '/api/v1/worker/auth/password', method: 'POST', data: { currentPassword, newPassword, confirmPassword } })
}

export async function logoutWorker(): Promise<void> {
  try {
    await request<{ loggedOut: boolean }>({ url: '/api/v1/worker/auth/logout', method: 'POST' })
  } catch (error) {
    if (!(error instanceof ApiError)) throw error
  } finally {
    clearWorkerSession()
  }
}
