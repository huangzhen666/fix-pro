import { useEffect, useState } from 'react'
import { Image, Typography } from 'antd'
import { apiBlob } from '../api/http'
export function AuthMedia({ url, type, name }: { url: string; type: 'IMAGE' | 'VIDEO'; name: string }) {
  const [media, setMedia] = useState({ url: '', src: '', failed: false })
  useEffect(() => {
    let objectUrl = '', cancelled = false
    apiBlob(url).then(blob => {
      if (cancelled) return
      objectUrl = URL.createObjectURL(blob); setMedia({ url, src: objectUrl, failed: false })
    }).catch(() => { if (!cancelled) setMedia({ url, src: '', failed: true }) })
    return () => { cancelled = true; if (objectUrl) URL.revokeObjectURL(objectUrl) }
  }, [url])
  if (media.url !== url || !media.src) return <Typography.Text type="secondary">{media.url === url && media.failed ? `${name}加载失败` : `正在加载 ${name}`}</Typography.Text>
  return type === 'IMAGE' ? <Image width={80} src={media.src} /> : <video controls src={media.src} style={{ width: 240, maxHeight: 180 }} />
}
