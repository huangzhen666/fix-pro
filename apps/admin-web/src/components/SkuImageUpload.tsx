import { useEffect, useRef, useState } from 'react'
import { App, Image, Upload } from 'antd'
import type { UploadFile, UploadProps } from 'antd'
import { apiBlob } from '../api/http'

export function SkuImageUpload(props: UploadProps) {
  const { message } = App.useApp()
  const sources = useRef(new Map<string, string>())
  const [imageUrls, setImageUrls] = useState<Record<string, string>>({})
  const [preview, setPreview] = useState({ src: '', name: '', open: false })
  const mediaIds = JSON.stringify([...new Set((props.fileList ?? [])
    .filter(file => file.status === 'done' && file.response?.id)
    .map(file => String(file.response.id)))])

  useEffect(() => {
    let cancelled = false
    const ids: string[] = JSON.parse(mediaIds)
    for (const [id, src] of sources.current) {
      if (!ids.includes(id)) {
        URL.revokeObjectURL(src)
        sources.current.delete(id)
      }
    }
    setImageUrls(Object.fromEntries(sources.current))
    for (const id of ids) {
      if (sources.current.has(id)) continue
      apiBlob(`/api/v1/admin/media/${encodeURIComponent(id)}/content`).then(blob => {
        if (cancelled) return
        sources.current.set(id, URL.createObjectURL(blob))
        setImageUrls(Object.fromEntries(sources.current))
      }).catch(() => {
        if (!cancelled) message.error('图片加载失败，请重新打开编辑页重试')
      })
    }
    return () => { cancelled = true }
  }, [mediaIds, message])

  useEffect(() => {
    const urls = sources.current
    return () => {
      for (const src of urls.values()) URL.revokeObjectURL(src)
      urls.clear()
    }
  }, [])

  const fileList = props.fileList?.map(file => {
    const src = imageUrls[String(file.response?.id)]
    return src ? { ...file, url: src, thumbUrl: src, type: 'image/*' } : file
  })

  function showPreview(file: UploadFile) {
    const src = imageUrls[String(file.response?.id)] || file.url || file.thumbUrl
    if (src) setPreview({ src, name: file.name, open: true })
    else message.info('图片正在加载，请稍后重试')
  }

  return <>
    <Upload {...props} fileList={fileList} onPreview={showPreview} />
    <Image
      src={preview.src}
      alt={preview.name}
      styles={{ root: { display: 'none' } }}
      preview={{ open: preview.open, onOpenChange: open => setPreview(current => ({ ...current, open })) }}
    />
  </>
}
