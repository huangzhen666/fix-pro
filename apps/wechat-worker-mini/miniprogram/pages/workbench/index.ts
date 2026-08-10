Page({
  data: {
    summary: [
      { label: '待接单', value: 0 },
      { label: '待上门', value: 0 },
      { label: '服务中', value: 0 },
      { label: '待完工', value: 0 },
    ],
  },
  onShow() {
    wx.setNavigationBarTitle({ title: '师傅工作台' })
  },
})
