import { useCallback, useEffect, useRef, useState } from 'react';
import { App, Alert, Button, Card, Checkbox, Collapse, Form, Input, InputNumber, Modal, Select, Space, Typography } from 'antd';
import type { UploadFile } from 'antd';
import { useQuery } from '@tanstack/react-query';
import { useNavigate, useParams } from 'react-router';
import { createSku, getSku, listCategories, listSkuTemplates, publishSku, updateSku, type SkuWrite, type SkuTemplate } from '../api/catalog';
import type { ServiceItem } from '../api/serviceProfile';
import { uploadFile } from '../api/http';
import { listSkills } from '../api/workforce';
import { SkuImageUpload } from '../components/SkuImageUpload';
import { ServiceProfileView } from '../components/ServiceProfileView';
type Values = Omit<SkuWrite, 'basePrice' | 'version'> & {
    priceYuan?: number;
};
const P = 'detailProfile';
const uploadEntry = (id: string): UploadFile => ({ uid: id, name: '服务图片', status: 'done', url: `/api/v1/admin/media/${id}/content`, response: { id } });
function ImageField({ value, onChange, id }: {
    id?: string;
    value?: string;
    onChange?: (v: string) => void;
}) {
    const { message } = App.useApp();
    return <div id={id}><SkuImageUpload listType="picture-card" maxCount={1} fileList={value ? [uploadEntry(value)] : []} onRemove={() => { onChange?.(''); return true; }} customRequest={async ({ file, onSuccess, onError }: any) => { try {
        const v = await uploadFile('/api/v1/admin/media/images', file as File);
        onChange?.(String(v.id));
        onSuccess(v);
    }
    catch (e) {
        message.error('图片上传失败');
        onError(e);
    } }}>上传图片</SkuImageUpload></div>;
}
export default function SkuEditPage() {
    const { id } = useParams(), navigate = useNavigate(), { message, modal } = App.useApp(), [form] = Form.useForm<Values>();
    const [openSections, setOpenSections] = useState<string[]>([]), [saving, setSaving] = useState(false), [template, setTemplate] = useState<SkuTemplate>(), [preview, setPreview] = useState(false), [confirming, setConfirming] = useState(false), [saved, setSaved] = useState<{
        id: string;
        version: number;
    }>();
    const initialized = useRef(false);
    const loadedDetail = useRef('');
    const cats = useQuery({ queryKey: ['categories'], queryFn: listCategories }), skills = useQuery({ queryKey: ['worker-skills-active'], queryFn: () => listSkills('', 'ACTIVE') }), templates = useQuery({ queryKey: ['sku-templates'], queryFn: listSkuTemplates }), detail = useQuery({ queryKey: ['sku', id], queryFn: () => getSku(id!), enabled: !!id });
    const values = Form.useWatch([], { form, preserve: true }) as Values | undefined, p = values?.detailProfile;
    const apply = useCallback((t: SkuTemplate) => { setTemplate(t); form.setFieldsValue({ unit: t.unit, description: undefined }); form.setFieldValue(P,structuredClone(t.profile)); }, [form]);
    useEffect(() => { if (detail.data) {
        const s = detail.data;
        if(loadedDetail.current!==`${s.id}:${s.version}`){
        form.resetFields();
        form.setFieldsValue({ categoryId: s.categoryId, name: s.name, description: s.description, priceMode: 'FIXED', priceYuan: s.basePrice / 100, unit: s.unit, requiredSkillIds: s.requiredSkillIds, coverMediaId: s.coverMediaId || '', galleryMediaIds: s.galleryMediaIds, detailProfile: s.detailProfile });
        form.setFieldValue('coverMediaId', s.coverMediaId || '');
        form.setFieldValue('galleryMediaIds', s.galleryMediaIds);
        setSaved({ id: s.id, version: s.version });
        loadedDetail.current=`${s.id}:${s.version}`;
        }
        setTemplate(templates.data?.find(t => t.key === s.detailProfile.templateKey));
        initialized.current = true;
    }
    else if (!id && templates.data && !initialized.current) {
        apply(templates.data[0]);
        initialized.current = true;
    } }, [detail.data, templates.data, id, form, apply]);
    const text = (name: any, label: string, required = false, area = false) => <Form.Item key={JSON.stringify(name)} name={name} label={label} rules={required ? [{ required: true, message: `请填写${label}` }] : []}>{area ? <Input.TextArea autoSize={{ minRows: 2, maxRows: 5 }}/> : <Input />}</Form.Item>;
    const strings = (path: any[], label: string) => <Form.List name={path}>{(fields, { add, remove }) => <div style={{ marginBottom: 16 }}><Typography.Text strong>{label}</Typography.Text>{fields.map(f => <Space key={f.key} align="start" style={{ display: 'flex', marginTop: 8 }}><Form.Item name={f.name} style={{ marginBottom: 8 }}><Input.TextArea autoSize placeholder={label}/></Form.Item><Button onClick={() => remove(f.name)}>移除</Button></Space>)}<Button size="small" onClick={() => add('')}>添加一项</Button></div>}</Form.List>;
    const items = (path: any[], label: string, photos = false) => <Form.List name={path}>{(fields, { add, remove }) => <div><Typography.Text strong>{label}</Typography.Text>{fields.map(f => <Card size="small" key={f.key} style={{ margin: '10px 0' }} extra={<Button size="small" onClick={() => remove(f.name)}>移除</Button>}>{text([f.name, 'title'], '项目名称')}{text([f.name, 'description'], '项目说明', false, true)}{photos && <Form.Item name={[f.name, 'imageMediaId']} label="项目图片（选填）"><ImageField /></Form.Item>}</Card>)}<Button onClick={() => add({ title: '', description: '' })}>添加{label}</Button></div>}</Form.List>;
    const objects = (key: string, label: string, cols: {
        key: string;
        label: string;
        image?: boolean;
        options?: string[];
    }[]) => <Form.List name={[P, 'enhancements', key]}>{(fields, { add, remove }) => <div><Typography.Text strong>{label}</Typography.Text>{fields.map(f => <Card size="small" key={f.key} style={{ margin: '10px 0' }} extra={<Button size="small" onClick={() => remove(f.name)}>移除</Button>}>{cols.map(c => <Form.Item key={c.key} name={[f.name, c.key]} label={c.label}>{c.image ? <ImageField /> : c.options ? <Select options={c.options.map(v => ({ value: v, label: { PROVIDER: '服务方提供', CUSTOMER: '客户准备', SHARED: '双方准备' }[v] || v }))}/> : <Input.TextArea autoSize/>}</Form.Item>)}</Card>)}<Button onClick={() => add({})}>添加{label}</Button></div>}</Form.List>;
    async function save(publish: boolean) { setSaving(true); try {
        if (publish)
            await form.validateFields();
        const v = form.getFieldsValue(true);
        const body: SkuWrite = { categoryId: v.categoryId, name: v.name || '未命名服务', description: v.description || '', priceMode: 'FIXED', unit: v.unit || '份', basePrice: Math.round((v.priceYuan ?? 0) * 100), requiredSkillIds: v.requiredSkillIds || [], coverMediaId: form.getFieldValue('coverMediaId') || '', galleryMediaIds: form.getFieldValue('galleryMediaIds') || [], detailProfile: v.detailProfile, version: saved?.version ?? detail.data?.version ?? 0 };
        const activeID = saved?.id || id;
        const result = activeID ? await updateSku(activeID, body) : await createSku(body);
        setSaved({ id: result.id, version: result.version });
        if (publish)
            await publishSku(result.id);
        message.success(publish ? '已发布' : '草稿已保存');
        navigate('/catalog/skus');
    }
    catch (e: any) {
        const expandField=(path:(string|number)[])=>{if(path.includes('enhancements'))setOpenSections(keys=>Array.from(new Set([...keys,'enhance'])));else if(path.includes('quantity')||path.includes('demandCollection'))setOpenSections(keys=>Array.from(new Set([...keys,'more'])));};
        if(e?.errorFields?.length){const path=e.errorFields[0].name;expandField(path);form.scrollToField(path);message.error('请补全标记的配置后再发布');}
        else {
            const raw=e instanceof Error?e.message:'保存失败';const colon=raw.indexOf(':');
            const key=colon>0?raw.slice(0,colon):'';
            const labels:Record<string,string>={detailProfile:'服务内容',serviceTarget:'服务对象',billing:'计价规则',includedItems:'服务项目',excludedItems:'不包含项目',unsuitableConditions:'不适用情况',purchaseNotices:'购买提示',acceptanceCriteria:'交付检查',afterSales:'售后政策',demandCollection:'需求资料设置',enhancements:'增强内容'};
            const parts=key.split('.');const label=labels[parts[0]];
            if(label){const path=parts.map(v=>/^\d+$/.test(v)?Number(v):v);expandField(path);form.setFields([{name:([P,...path] as Parameters<typeof form.setFields>[0][number]['name']),errors:[raw.slice(colon+1).trim()]}]);message.error(label+'：'+raw.slice(colon+1).trim());}
            else message.error(raw);
        }
    }
    finally {
        setSaving(false);
        setConfirming(false);
    } }
    const q = p?.billing?.quantity;
    return <div><Space style={{ marginBottom: 16 }}><Typography.Title level={3}>{id ? '编辑服务' : '新增服务'}</Typography.Title><Button className="sp-preview-toggle" onClick={() => setPreview(!preview)}>{preview ? '收起预览' : '查看手机预览'}</Button></Space>{detail.data && <Typography.Paragraph type="secondary">编码 {detail.data.skuCode} · 编辑版本 {detail.data.version}</Typography.Paragraph>}
 {templates.error && <Alert type="error" message="模板加载失败，请刷新后重试"/>}
 <div className={`sp-editor-layout ${preview ? 'preview-mode' : ''}`}><Card loading={detail.isLoading}><Space style={{ marginBottom: 20 }}><span>服务模板</span><Select style={{ width: 220 }} value={template?.key} placeholder="选择模板" options={templates.data?.map(t => ({ value: t.key, label: t.name }))} onChange={key => { const t = templates.data?.find(x => x.key === key); if (!t)
        return; modal.confirm({ title: '应用服务模板', content: '将替换服务内容、数量和需求规则、交付与售后建议。名称、分类、价格及素材保留，请在发布前核对。', onOk: () => { const current = form.getFieldsValue(true); apply(t); form.setFieldsValue({ name: current.name, categoryId: current.categoryId, priceYuan: current.priceYuan }); const old = current.detailProfile; if (old) {
            form.setFieldValue([P, 'enhancements'], old.enhancements);
            const images = new Map(old.includedItems?.map((v: ServiceItem) => [v.title, v.imageMediaId]));
            form.setFieldValue([P, 'includedItems'], (t.profile.includedItems || []).map(v => ({ ...v, imageMediaId: images.get(v.title) })));
        } } }); }}/></Space>
 <Form form={form} layout="vertical" initialValues={{ priceMode: 'FIXED' }}>
 <Typography.Title level={4}>① 服务信息</Typography.Title>{text('name', '服务名称', true)}<Space align="start"><Form.Item name="categoryId" label="分类" rules={[{ required: true }]}><Select style={{ width: 210 }} options={cats.data?.map(c => ({ value: c.id, label: c.name }))}/></Form.Item><Form.Item name="priceYuan" label="价格（元）" rules={[{ required: true, type: 'number', min: 0.01, message: '发布前请填写有效价格' }]}><InputNumber min={0.01} max={999999.99} precision={2}/></Form.Item>{text('unit', '销售单位', true)}</Space>{text('description', '简述（选填）')}
 <Typography.Title level={4}>② 服务内容</Typography.Title>{text([P, 'serviceTarget'], '服务对象与适用范围', true)}{template?.candidates?.length ? <Checkbox.Group style={{ marginBottom: 16 }} value={(p?.includedItems || []).filter(v => template.candidates.some(c => c.title === v.title)).map(v => v.title)} options={template.candidates.map(v => ({ label: v.title, value: v.title }))} onChange={vs => { const custom = (p?.includedItems || []).filter(v => !template.candidates.some(c => c.title === v.title)); form.setFieldValue([P, 'includedItems'], [...template.candidates.filter(v => vs.includes(v.title)).map(v => p?.includedItems?.find(old => old.title === v.title) || v), ...custom]); }}/> : null}{items([P, 'includedItems'], '服务项目', true)}
 <Typography.Title level={4}>③ 计价与边界</Typography.Title>{text([P, 'billing', 'unitDefinition'], '一份服务包含什么', true)}{text([P, 'billing', 'quantityExplanation'], '购买数量说明', true, true)}<Form.Item name={[P, 'billing', 'visitFeeMode']} label="上门费用"><Select options={[{ value: 'INCLUDED', label: '已含上门费' }, { value: 'WAIVED', label: '免收上门费' }, { value: 'NOT_APPLICABLE', label: '不适用' }]}/></Form.Item>{text([P, 'billing', 'feeDescription'], '费用与材料责任说明', true, true)}{items([P, 'excludedItems'], '不包含项目')}{strings([P, 'unsuitableConditions'], '不适用情况（选填）')}{strings([P, 'purchaseNotices'], '购买提示（选填）')}
 <Typography.Title level={4}>④ 交付与售后</Typography.Title>{strings([P, 'acceptanceCriteria'], '交付检查项')}<Form.Item name={[P, 'afterSales', 'type']} label="售后政策"><Select options={[{ value: 'SUPPORT', label: '一般售后支持' }, { value: 'REWORK', label: '复洁或重做政策' }, { value: 'WARRANTY', label: '期限质保' }]}/></Form.Item>{text([P, 'afterSales', 'summary'], '政策摘要', true)}{text([P, 'afterSales', 'coverage'], '具体处理范围', true, true)}{(p?.afterSales?.type === 'WARRANTY' || p?.afterSales?.period) && <><Space><Form.Item name={[P, 'afterSales', 'period', 'value']} label="期限"><InputNumber min={1} max={3650}/></Form.Item><Form.Item name={[P, 'afterSales', 'period', 'unit']} label="时间单位"><Select style={{ width: 100 }} options={[{ value: 'HOUR', label: '小时' }, { value: 'DAY', label: '天' }, { value: 'MONTH', label: '个月' }]}/></Form.Item></Space>{text([P, 'afterSales', 'period', 'startDescription'], '起算方式', p?.afterSales?.type === 'WARRANTY')}<Button onClick={() => form.setFieldValue([P, 'afterSales', 'period'], undefined)}>移除期限</Button></>}{p?.afterSales?.type !== 'WARRANTY' && !p?.afterSales?.period && <Button onClick={() => form.setFieldValue([P, 'afterSales', 'period'], { value: 24, unit: 'HOUR', startDescription: '' })}>添加政策反馈期限（选填）</Button>}{strings([P, 'afterSales', 'exclusions'], '售后不包含（选填）')}{text([P, 'afterSales', 'contactDescription'], '如何联系', true)}
 <Typography.Title level={4}>⑤ 服务主图</Typography.Title><Form.Item name="coverMediaId" label="服务主图" rules={[{ required: true }]}><ImageField /></Form.Item>
 <Alert style={{ margin: '16px 0' }} type="info" message={`当前规则：${q?.min ?? '—'}–${q?.max ?? '—'} ${values?.unit || '份'}，步长 ${q?.step ?? '—'}；${q?.aggregation === 'ADDITIVE' ? '累计额度' : '独立对象'}。需求文字${p?.demandCollection?.description?.mode === 'REQUIRED' ? '必填' : '非必填'}，现场资料${p?.demandCollection?.media?.mode === 'REQUIRED' ? '必填' : '非必填'}`} description="请确认当前规则适合服务；需要调整可展开更多设置。"/>
 <Collapse activeKey={openSections} onChange={keys => setOpenSections(typeof keys === 'string' ? [keys] : keys)} items={[{ key: 'more', label: '更多设置 · 数量、需求资料与技能', forceRender: true, children: <><Space>{(['min', 'max', 'step'] as const).map((k) => <Form.Item key={k} name={[P, 'billing', 'quantity', k]} label={{ min: '最少数量', max: '最多数量', step: '步长' }[k]}><InputNumber min={1} max={9999}/></Form.Item>)}</Space><Form.Item name={[P, 'billing', 'quantity', 'aggregation']} label="数量含义"><Select options={[{ value: 'INDEPENDENT', label: '每单位为独立对象' }, { value: 'ADDITIVE', label: '小时/面积等额度累计' }]}/></Form.Item>{(['description', 'media'] as const).map(k => <div key={k}><Form.Item name={[P, 'demandCollection', k, 'mode']} label={k === 'description' ? '需求文字' : '现场图片/视频'}><Select options={[{ value: 'OPTIONAL', label: '选填' }, { value: 'REQUIRED', label: '必填' }, { value: 'HIDDEN', label: '不收集' }]}/></Form.Item>{p?.demandCollection?.[k]?.mode !== 'HIDDEN' && <>{text([P, 'demandCollection', k, 'label'], '客户看到的名称')}{text([P, 'demandCollection', k, 'hint'], '填写提示')}</>}</div>)}<Form.Item name="requiredSkillIds" label="建议技能"><Select mode="multiple" allowClear options={skills.data?.map(s => ({ value: s.id, label: s.name }))}/></Form.Item></> }, { key: 'enhance', label: '增强内容 · 全部选填', forceRender: true, children: <>{strings([P, 'enhancements', 'tags'], '场景标签')}{objects('facts', '服务参数', [{ key: 'label', label: '参数名' }, { key: 'value', label: '参数值' }])}{objects('resources', '工具材料责任', [{ key: 'name', label: '名称' }, { key: 'provider', label: '提供方', options: ['PROVIDER', 'CUSTOMER', 'SHARED'] }, { key: 'description', label: '责任说明' }])}{strings([P, 'enhancements', 'preparations'], '服务前准备')}{items([P, 'enhancements', 'steps'], '流程步骤')}{objects('gallery', '服务案例', [{ key: 'mediaId', label: '案例图片', image: true }, { key: 'caption', label: '图片说明' }])}{objects('comparisons', '前后对照', [{ key: 'title', label: '标题' }, { key: 'beforeMediaId', label: '服务前', image: true }, { key: 'afterMediaId', label: '服务后', image: true }, { key: 'description', label: '服务说明' }, { key: 'effectNotice', label: '效果适用提示' }])}{objects('faqs', '常见问题', [{ key: 'question', label: '问题' }, { key: 'answer', label: '回答' }])}</> }]}/>
 <Space style={{ marginTop: 24 }}><Button onClick={() => navigate('/catalog/skus')}>取消</Button><Button loading={saving} onClick={() => save(false)}>保存草稿</Button><Button type="primary" loading={saving} onClick={() => setConfirming(true)}>核对并发布</Button></Space>
 </Form></Card><aside className={`sp-preview ${preview ? 'mobile-visible' : ''}`}><small>草稿手机预览 · 未发布</small>{form.getFieldValue('coverMediaId') && <img className="sp-hero" src={`/api/v1/admin/media/${form.getFieldValue('coverMediaId')}/content`} alt="服务主图"/>}<h2>{values?.name || '服务名称'}</h2><p>{values?.description}</p><div className="sp-preview-price">¥{Number(values?.priceYuan || 0).toFixed(2)} / {values?.unit || '份'}</div><p>{p?.billing?.unitDefinition}</p><ServiceProfileView profile={p}/></aside></div>
 <Modal title="核对服务承诺并发布" open={confirming} onCancel={() => setConfirming(false)} onOk={() => save(true)} confirmLoading={saving} okText="确认并发布" width={620}><p>价格 ¥{values?.priceYuan} / {values?.unit}；{q?.min}–{q?.max}，步长{q?.step}</p><p>{p?.billing?.unitDefinition}</p><p>{p?.billing?.quantityExplanation}</p><p>{p?.billing?.feeDescription}</p><p>需求文字：{{ HIDDEN: '不收集', OPTIONAL: '选填', REQUIRED: '必填' }[p?.demandCollection?.description?.mode || 'OPTIONAL']}；现场资料：{{ HIDDEN: '不收集', OPTIONAL: '选填', REQUIRED: '必填' }[p?.demandCollection?.media?.mode || 'OPTIONAL']}</p><ServiceProfileView profile={p}/></Modal>
 </div>;
}
