import { statusLabel } from '../../services/status'
Page({data:{orderNo:'',status:'',statusText:'',amountText:'0.00'},onLoad(options){const status=String(options.status||'');this.setData({orderNo:String(options.orderNo||''),status,statusText:statusLabel(status),amountText:(Number(options.amount||0)/100).toFixed(2)})},goServices(){wx.switchTab({url:'/pages/services/index'})}})
