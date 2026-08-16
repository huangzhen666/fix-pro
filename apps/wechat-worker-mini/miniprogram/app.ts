App({
  onLaunch() {},
  onShow() {
    const token = wx.getStorageSync<string>('fixpro.worker.accessToken')
    const pages = getCurrentPages()
    const route = pages.length ? pages[pages.length - 1].route : ''
    if (!token && route && route !== 'pages/login/index' && route !== 'pages/change-password/index') {
      wx.reLaunch({ url: '/pages/login/index' })
    }
  },
})
