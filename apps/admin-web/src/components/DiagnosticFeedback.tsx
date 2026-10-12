import { useEffect, useState } from 'react'
import { Alert, Button, Typography } from 'antd'
import { diagnosticText, type Diagnostic } from '../api/diagnostics'

export default function DiagnosticFeedback() {
  const [error, setError] = useState<Diagnostic>()
  useEffect(() => {
    const handle = (event: Event) => setError((event as CustomEvent<Diagnostic>).detail)
    window.addEventListener('fixpro:api-error', handle)
    return () => window.removeEventListener('fixpro:api-error', handle)
  }, [])
  if (!error) return null
  return <div style={{ position: 'fixed', right: 20, bottom: 20, maxWidth: 420, zIndex: 1100 }}>
    <Alert type="warning" title="操作未完成" closable onClose={() => setError(undefined)} description={
      <details><summary>错误详情</summary><Typography.Paragraph style={{ whiteSpace: 'pre-wrap', marginTop: 8 }} copyable={{ text: diagnosticText(error) }}>{diagnosticText(error)}</Typography.Paragraph>
        <Button size="small" onClick={() => setError(undefined)}>关闭</Button>
      </details>
    } />
  </div>
}
