type MiniProgramEnv = 'develop' | 'trial' | 'release'

const apiBaseUrls: Record<MiniProgramEnv, string> = {
  develop: 'http://localhost:8080',
  trial: 'https://staging-api.example.com',
  release: 'https://api.example.com',
}

export function getApiBaseUrl(): string {
  const envVersion = wx.getAccountInfoSync().miniProgram.envVersion as MiniProgramEnv
  return apiBaseUrls[envVersion] ?? apiBaseUrls.develop
}
