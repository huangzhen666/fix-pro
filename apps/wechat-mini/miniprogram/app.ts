interface FixProAppOptions {
  globalData: {
    role: 'CUSTOMER' | 'WORKER'
  }
}

App<FixProAppOptions>({
  globalData: {
    role: 'CUSTOMER',
  },
  onLaunch() {
    if (wx.getAccountInfoSync().miniProgram.envVersion === 'develop' && !wx.getStorageSync('fixpro.accessToken')) {
      wx.setStorageSync('fixpro.accessToken', 'local-customer-1')
    }
    const savedRole = wx.getStorageSync<'CUSTOMER' | 'WORKER'>('fixpro.role')
    if (savedRole === 'CUSTOMER' || savedRole === 'WORKER') {
      this.globalData.role = savedRole
    }
  },
})
