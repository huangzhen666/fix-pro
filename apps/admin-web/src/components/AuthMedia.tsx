import { useEffect, useState } from 'react'
import { Image, Typography } from 'antd'
import { apiBlob } from '../api/http'

export function AuthMedia({ url, type, name }: { url: string; type: 'IMAGE' | 'VIDEO'; name: string }) {
  const [src, setSrc] = useState('')
  useEffect(() => { let objectUrl=''; apiBlob(url).then(blob=>{objectUrl=URL.createObjectURL(blob);setSrc(objectUrl)}); return()=>{if(objectUrl)URL.revokeObjectURL(objectUrl)} }, [url])
  if(!src)return <Typography.Text type="secondary">正在加载 {name}</Typography.Text>
  return type==='IMAGE'?<Image width={120} src={src}/>:<video controls src={src} style={{width:240,maxHeight:180}} />
}
