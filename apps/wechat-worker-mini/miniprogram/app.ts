import { setWorkerToken } from './services/request'

App({
  onLaunch() {
    if (!wx.getStorageSync('fixpro.worker.accessToken')) {
      setWorkerToken('local-worker-1')
    }
  },
})
