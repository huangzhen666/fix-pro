import { useState } from 'react'
import { App, Button, Card, Form, Input, Modal, Popconfirm, Space, Table, Tag, Typography } from 'antd'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { createSkill, createTrade, deleteSkill, deleteTrade, listSkills, listTrades, setSkillStatus, setTradeStatus, updateSkill, updateTrade } from '../api/workforce'
import { statusLabel } from '../utils/enums'

export default function SkillPage() {
  const { message } = App.useApp()
  const qc = useQueryClient()
  const [tradeId, setTradeId] = useState('')
  const [tradeForm] = Form.useForm()
  const [skillForm] = Form.useForm()
  const [tradeEditing, setTradeEditing] = useState<any>(null)
  const [skillEditing, setSkillEditing] = useState<any>(null)
  const trades = useQuery({ queryKey: ['worker-trades'], queryFn: () => listTrades('') })
  const skills = useQuery({ queryKey: ['worker-skills', tradeId], queryFn: () => listSkills(tradeId, ''), enabled: Boolean(tradeId) })

  function openTrade(row?: any) {
    setTradeEditing(row ?? {})
    tradeForm.setFieldsValue(row ? { name: row.name, description: row.description } : {})
  }
  function openSkill(row?: any) {
    setSkillEditing(row ?? { tradeId })
    skillForm.setFieldsValue(row ? { name: row.name, description: row.description } : {})
  }
  async function saveTrade() {
    try {
      const values = await tradeForm.validateFields()
      if (tradeEditing?.id) await updateTrade(tradeEditing.id, { ...tradeEditing, ...values })
      else await createTrade(values)
      message.success('工种已保存'); setTradeEditing(null); tradeForm.resetFields(); qc.invalidateQueries({ queryKey: ['worker-trades'] })
    } catch (e) { if (e instanceof Error) message.error(e.message) }
  }
  async function saveSkill() {
    try {
      const values = await skillForm.validateFields()
      if (skillEditing?.id) await updateSkill(skillEditing.id, { ...skillEditing, ...values })
      else await createSkill({ ...values, tradeId })
      message.success('技能已保存'); setSkillEditing(null); skillForm.resetFields(); qc.invalidateQueries({ queryKey: ['worker-skills', tradeId] }); qc.invalidateQueries({ queryKey: ['worker-trades'] })
    } catch (e) { if (e instanceof Error) message.error(e.message) }
  }
  return <Card><Space direction="vertical" style={{ width: '100%' }} size="large">
    <Space><Typography.Title level={3} style={{ margin: 0 }}>工种与技能</Typography.Title><Button onClick={() => openTrade()}>新增工种</Button><Button type="primary" disabled={!tradeId} onClick={() => openSkill()}>为选中工种新增技能</Button></Space>
    <Space align="start"><Table rowKey="id" size="small" dataSource={trades.data} rowSelection={{ type: 'radio', selectedRowKeys: tradeId ? [tradeId] : [], onChange: keys => setTradeId(String(keys[0] ?? '')) }} columns={[
      { title: '工种编码', dataIndex: 'tradeCode' }, { title: '工种', dataIndex: 'name' }, { title: '技能数量', dataIndex: 'skillCount' }, { title: '状态', dataIndex: 'status', render: (v: string) => <Tag>{statusLabel(v)}</Tag> },
      { title: '操作', render: (_: unknown, row: any) => <Space><Button type="link" onClick={() => openTrade(row)}>编辑</Button><Button type="link" onClick={async () => { try { await setTradeStatus(row.id, row.status === 'ACTIVE' ? 'DISABLED' : 'ACTIVE', row.version); qc.invalidateQueries({ queryKey: ['worker-trades'] }) } catch (e) { if (e instanceof Error) message.error(e.message) } }}>{row.status === 'ACTIVE' ? '禁用' : '启用'}</Button><Popconfirm title="确认删除工种？" description="只有工种下没有任何技能时才能删除。" okText="确认删除" cancelText="取消" onConfirm={async () => { try { await deleteTrade(row.id); message.success('工种已删除'); if (tradeId === row.id) setTradeId(''); qc.invalidateQueries({ queryKey: ['worker-trades'] }) } catch (e) { if (e instanceof Error) message.error(e.message) } }}><Button type="link" danger>删除</Button></Popconfirm></Space> }
    ]} pagination={false} />
    <Table rowKey="id" size="small" dataSource={skills.data} columns={[
      { title: '所属工种', dataIndex: 'tradeName' }, { title: '技能', dataIndex: 'name' }, { title: '系统编码', dataIndex: 'skillCode' }, { title: '状态', dataIndex: 'status', render: (v: string) => statusLabel(v) },
      { title: '操作', render: (_: unknown, row: any) => <Space><Button type="link" onClick={() => openSkill(row)}>编辑</Button><Button type="link" onClick={async () => { try { await setSkillStatus(row.id, row.status === 'ACTIVE' ? 'DISABLED' : 'ACTIVE', row.version); qc.invalidateQueries({ queryKey: ['worker-skills', tradeId] }) } catch (e) { if (e instanceof Error) message.error(e.message) } }}>{row.status === 'ACTIVE' ? '禁用' : '启用'}</Button><Popconfirm title="确认删除技能？" description="已被师傅或 SKU 使用的技能不能删除。" okText="确认删除" cancelText="取消" onConfirm={async () => { try { await deleteSkill(row.id); message.success('技能已删除'); qc.invalidateQueries({ queryKey: ['worker-skills', tradeId] }); qc.invalidateQueries({ queryKey: ['worker-trades'] }) } catch (e) { if (e instanceof Error) message.error(e.message) } }}><Button type="link" danger>删除</Button></Popconfirm></Space> }
    ]} pagination={false} /></Space>
    <Modal open={tradeEditing !== null} title={tradeEditing?.id ? '编辑工种' : '新增工种'} onCancel={() => setTradeEditing(null)} onOk={saveTrade}><Form form={tradeForm} layout="vertical"><Form.Item label="系统编码"> <Input value={tradeEditing?.tradeCode ?? '保存后由系统自动生成'} disabled /></Form.Item><Form.Item name="name" label="工种名称" rules={[{ required: true, min: 2, max: 64 }]}><Input /></Form.Item><Form.Item name="description" label="说明"><Input.TextArea maxLength={500} /></Form.Item></Form></Modal>
    <Modal open={skillEditing !== null} title={skillEditing?.id ? '编辑技能' : '新增技能'} onCancel={() => setSkillEditing(null)} onOk={saveSkill}><Form form={skillForm} layout="vertical"><Form.Item label="所属工种"><Input value={skillEditing?.tradeName ?? trades.data?.find(x => x.id === tradeId)?.name} disabled /></Form.Item><Form.Item label="系统编码"><Input value={skillEditing?.skillCode ?? '保存后由系统自动生成'} disabled /></Form.Item><Form.Item name="name" label="技能名称" rules={[{ required: true, min: 2, max: 64 }]}><Input /></Form.Item><Form.Item name="description" label="说明"><Input.TextArea maxLength={500} /></Form.Item></Form></Modal>
  </Space></Card>
}
