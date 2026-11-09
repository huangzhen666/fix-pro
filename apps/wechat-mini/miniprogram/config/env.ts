type MiniProgramEnv = 'develop' | 'trial' | 'release'

const apiBaseUrls: Record<MiniProgramEnv, string> = {
  develop: 'http://localhost:8080',
  trial: 'https://staging-api.example.com',
  release: 'https://api.example.com',
}

export function isDevtools(): boolean {
 try { return wx.getDeviceInfo().platform === 'devtools' } catch { return false }
}

export function getApiBaseUrl(): string {
  const envVersion = wx.getAccountInfoSync().miniProgram.envVersion as MiniProgramEnv
  if (envVersion==='develop' && !isDevtools()) return apiBaseUrls.trial
  return apiBaseUrls[envVersion] ?? apiBaseUrls.trial
}
