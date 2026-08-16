import { useState } from 'react'
import { App, Button, Card, Form, Input, Modal, Select, Space, Table, Tag, Typography, Upload } from 'antd'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { getWorker, listSkills, listTrades, listWorkers, saveWorker, disableWorker, resetWorkerPassword, uploadWorkerMedia, type WorkerMedia, type WorkerWrite } from '../api/workforce'
import { statusLabel } from '../utils/enums'
import { AuthMedia } from '../components/AuthMedia'

function uploadedMedia(result: { id: string; mediaType: string }, name: string): WorkerMedia {
  return { id: String(result.id), mediaType: result.mediaType === 'VIDEO' ? 'VIDEO' : 'IMAGE', contentType: '', name, url: `/api/v1/admin/media/${result.id}/content`, createdAt: new Date().toISOString() }
}

export default function WorkerPage() {
  const { message } = App.useApp()
  const [form] = Form.useForm<WorkerWrite>()
  const [disableForm] = Form.useForm()
  const [editing, setEditing] = useState<any>(null)
  const [keyword, setKeyword] = useState('')
  const [disable, setDisable] = useState<any>(null)
  const [avatar, setAvatar] = useState<WorkerMedia>()
  const [certificates, setCertificates] = useState<WorkerMedia[]>([])
  const [uploading, setUploading] = useState(false)
  const qc = useQueryClient()
  const workers = useQuery({ queryKey: ['workers-admin', keyword], queryFn: () => listWorkers('', keyword) })
  const trades = useQuery({ queryKey: ['worker-trades'], queryFn: () => listTrades('ACTIVE') })
  const skills = useQuery({ queryKey: ['worker-skills'], queryFn: () => listSkills('', 'ACTIVE') })

  async function open(row?: any) {
    if (!row) {
      setEditing({})
      setAvatar(undefined)
      setCertificates([])
      form.setFieldsValue({ tradeIds: [], skillIds: [], version: 0, avatarMediaId: 0, certificateMediaIds: [] })
      return
    }
    try {
      const detail = await getWorker(row.id)
      setEditing(detail)
      setAvatar(detail.avatar)
      setCertificates(detail.certificates ?? [])
      form.setFieldsValue({ ...detail, tradeIds: detail.tradeIds ?? [], skillIds: detail.skillIds ?? [], version: detail.version, avatarMediaId: detail.avatar ? Number(detail.avatar.id) : 0, certificateMediaIds: (detail.certificates ?? []).map(item => Number(item.id)) })
    } catch (e) {
      if (e instanceof Error) message.error(e.message)
    }
  }

  async function uploadAvatar(file: File) {
    try {
      setUploading(true)
      const result = await uploadWorkerMedia('AVATAR', file)
      setAvatar(uploadedMedia(result, file.name))
      message.success('师傅照片上传成功')
    } catch (e) {
      if (e instanceof Error) message.error(e.message)
    } finally {
      setUploading(false)
    }
  }

  async function uploadCertificate(file: File) {
    try {
      setUploading(true)
      const result = await uploadWorkerMedia('CERTIFICATE', file)
      setCertificates(current => [...current, uploadedMedia(result, file.name)])
      message.success('技能证书上传成功')
    } catch (e) {
      if (e instanceof Error) message.error(e.message)
    } finally {
      setUploading(false)
    }
  }

  async function submit(activate = false) {
    try {
      const values = await form.validateFields()
      const result = await saveWorker(editing?.id, { ...values, activate, version: editing?.version ?? 0, avatarMediaId: avatar ? Number(avatar.id) : 0, certificateMediaIds: certificates.map(item => Number(item.id)) })
      message.success(activate ? '师傅已保存并启用' : '草稿已保存')
      setEditing(null)
      qc.invalidateQueries({ queryKey: ['workers-admin'] })
      if (result.initialPassword) {
        Modal.success({ title: '师傅账号已创建', content: `初始密码：${result.initialPassword}。请通过安全渠道交给师傅，关闭后不再显示。` })
      }
    } catch (e) {
      if (e instanceof Error) message.error(e.message)
    }
  }

  return <Card>
    <Space direction="vertical" style={{ width: '100%' }} size="large">
      <Space><Typography.Title level={3} style={{ margin: 0 }}>师傅管理</Typography.Title><Button type="primary" onClick={() => open()}>新增师傅</Button><Input.Search allowClear placeholder="编号、姓名或手机号" onSearch={setKeyword} style={{ width: 260 }} /></Space>
      <Table rowKey="id" loading={workers.isLoading} dataSource={workers.data} columns={[{ title: '系统编号', dataIndex: 'workerNo' }, { title: '姓名', dataIndex: 'displayName' }, { title: '手机号', dataIndex: 'mobileMasked' }, { title: '状态', dataIndex: 'status', render: (v: string) => <Tag color={v === 'ACTIVE' ? 'green' : 'default'}>{statusLabel(v)}</Tag> }, { title: '首次改密', render: (_: unknown, row: any) => <Tag color={row.mustChangePassword ? 'orange' : 'green'}>{row.mustChangePassword ? '待改密' : '已完成'}</Tag> }, { title: '未完成工单', dataIndex: 'openWorkOrderCount' }, { title: '操作', render: (_: unknown, row: any) => <Space><Button type="link" onClick={() => open(row)}>编辑</Button><Button type="link" onClick={() => Modal.confirm({ title: '重置师傅密码', content: '重置后该师傅现有登录将失效，并需要首次登录改密，是否继续？', okText: '确认重置', cancelText: '取消', onOk: async () => { try { const result = await resetWorkerPassword(row.id); Modal.success({ title: '密码已重置', content: `初始密码：${result.temporaryPassword}。请通过安全渠道交给师傅，关闭后不再显示。` }); qc.invalidateQueries({ queryKey: ['workers-admin'] }) } catch (e) { if (e instanceof Error) message.error(e.message); throw e } } })}>重置密码</Button>{row.status === 'ACTIVE' ? <Button type="link" danger onClick={() => { setDisable(row); disableForm.setFieldsValue({ workOrderPolicy: 'KEEP_ASSIGNMENTS', reason: '' }) }}>停用</Button> : null}</Space> }]} />
    </Space>
    <Modal open={!!editing} title={editing?.id ? '编辑师傅' : '新增师傅'} onCancel={() => setEditing(null)} footer={[<Button key="cancel" onClick={() => setEditing(null)}>取消</Button>, <Button key="save" onClick={() => submit(false)}>保存草稿</Button>, <Button key="active" type="primary" onClick={() => submit(true)}>保存并启用</Button>]}>
      <Form form={form} layout="vertical">
        <Form.Item label="系统编号"><Input value={editing?.workerNo ?? '保存后由系统自动生成'} disabled /></Form.Item>
        <Form.Item name="displayName" label="姓名" rules={[{ required: true, min: 2 }]}><Input /></Form.Item>
        <Form.Item name="mobile" label="手机号" rules={[{ required: true, len: 11 }]}><Input /></Form.Item>
        <Form.Item name="tradeIds" label="工种"><Select mode="multiple" options={trades.data?.map(x => ({ value: Number(x.id), label: x.name }))} /></Form.Item>
        <Form.Item name="skillIds" label="技能"><Select mode="multiple" options={skills.data?.map(x => ({ value: Number(x.id), label: `${x.tradeName ?? ''} / ${x.name}` }))} /></Form.Item>
        <Form.Item label="师傅照片（选填）">
          <Space direction="vertical">
            {avatar ? <Space align="start"><AuthMedia url={avatar.url} type={avatar.mediaType} name={avatar.name} /><Button type="link" danger onClick={() => setAvatar(undefined)}>移除照片</Button></Space> : <Typography.Text type="secondary">暂未上传</Typography.Text>}
            <Upload accept="image/png,image/jpeg,image/webp" showUploadList={false} beforeUpload={file => { void uploadAvatar(file); return Upload.LIST_IGNORE }}><Button loading={uploading}>上传照片</Button></Upload>
          </Space>
        </Form.Item>
        <Form.Item label="技能证书附件（选填，可多选）">
          <Space direction="vertical" style={{ width: '100%' }}>
            {certificates.map(item => <Space key={item.id} align="start"><AuthMedia url={item.url} type={item.mediaType} name={item.name} /><Button type="link" danger onClick={() => setCertificates(current => current.filter(x => x.id !== item.id))}>移除附件</Button></Space>)}
            {!certificates.length ? <Typography.Text type="secondary">暂未上传</Typography.Text> : null}
            <Upload accept="image/png,image/jpeg,image/webp" multiple showUploadList={false} beforeUpload={file => { void uploadCertificate(file); return Upload.LIST_IGNORE }}><Button loading={uploading}>上传证书附件</Button></Upload>
          </Space>
        </Form.Item>
        <Form.Item name="joinedOn" label="入职日期"><Input type="date" /></Form.Item>
        <Form.Item name="remark" label="备注"><Input.TextArea maxLength={500} /></Form.Item>
      </Form>
    </Modal>
    <Modal open={!!disable} title="停用师傅" onCancel={() => setDisable(null)} onOk={async () => { try { const v = await disableForm.validateFields(); await disableWorker(disable.id, { reason: v.reason, workOrderPolicy: v.workOrderPolicy, version: disable.version }); message.success('已停用'); setDisable(null); qc.invalidateQueries({ queryKey: ['workers-admin'] }) } catch (e) { if (e instanceof Error) message.error(e.message) } }}>
      <Typography.Paragraph>停用只阻止新派单；服务中和已到达工单不会自动改派。</Typography.Paragraph>
      <Form form={disableForm} layout="vertical"><Form.Item name="workOrderPolicy" label="已有预约处理"><Select options={[{ value: 'KEEP_ASSIGNMENTS', label: '保留已有预约（默认）' }, { value: 'RETURN_NOT_STARTED', label: '退回待接/待上门工单' }]} /></Form.Item><Form.Item name="reason" label="停用原因" rules={[{ required: true }]}><Input.TextArea /></Form.Item></Form>
    </Modal>
  </Card>
}
