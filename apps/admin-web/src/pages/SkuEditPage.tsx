import { useEffect, useRef, useState } from 'react'
import { Alert, AutoComplete, App, Button, Card, Form, Input, Select, Space, Typography } from 'antd'
import type { UploadFile } from 'antd'
import { useQuery } from '@tanstack/react-query'
import { useBeforeUnload, useBlocker, useNavigate, useParams, useSearchParams } from 'react-router'
import { createSku, getSku, listCategories, publishSku, updateSku, type SkuWrite } from '../api/catalog'
import { ApiError, apiRequest, uploadFile } from '../api/http'
import { SkuImageUpload } from '../components/SkuImageUpload'
import { ServiceContent } from '../components/ServiceContent'
type Issue = { field:string; message:string }
const entry=(id:string):UploadFile=>({uid:id,name:'服务图片',status:'done',response:{id}})
export default function SkuEditPage(){
 const {id}=useParams(),[params]=useSearchParams(),navigate=useNavigate(),{message,modal}=App.useApp(),[form]=Form.useForm()
 const [files,setFiles]=useState<UploadFile[]>([]),[issues,setIssues]=useState<Issue[]>([]),[busy,setBusy]=useState(false),[preview,setPreview]=useState(false),[dirty,setDirty]=useState(false)
 const dirtyRef=useRef(false),saved=useRef<{id:string;version:number}|undefined>(undefined),loaded=useRef('')
 const categories=useQuery({queryKey:['categories'],queryFn:listCategories}),detail=useQuery({queryKey:['sku',id],queryFn:()=>getSku(id!),enabled:!!id})
 const published=useQuery({queryKey:['sku-published',id],queryFn:()=>apiRequest<{price:number;unit:string;serviceDescription:string;serviceNotice:string}>(`/api/v1/catalog/services/${id}`),enabled:!!id&&!!detail.data?.publishedVersion,retry:false})
 const values=Form.useWatch([],{form,preserve:true})||{}
 const blocker=useBlocker(()=>dirtyRef.current)
 const change=()=>{dirtyRef.current=true;setDirty(true)}
 useBeforeUnload(event=>{if(dirtyRef.current){event.preventDefault();event.returnValue=''}})
 useEffect(()=>{if(blocker.state==='blocked'){const m=modal.confirm({title:'有尚未保存的修改',content:'离开后未保存的内容将丢失。',okText:'离开',cancelText:'继续编辑',onOk:()=>blocker.proceed(),onCancel:()=>blocker.reset()});return()=>m.destroy()}},[blocker,modal])
 useEffect(()=>{
  const s=detail.data;if(s&&loaded.current!==`${s.id}:${s.version}`){
   if(dirtyRef.current)return
   form.setFieldsValue({...s,priceYuan:s.basePrice?String(s.basePrice/100):''});setFiles([...(s.coverMediaId?[s.coverMediaId]:[]),...s.galleryMediaIds].map(entry));saved.current={id:s.id,version:s.version};loaded.current=`${s.id}:${s.version}`
  }else if(!id&&!loaded.current){form.setFieldValue('categoryId',params.get('categoryId')||undefined);loaded.current='new'}
 },[detail.data,form,id,params])
 function showIssues(next:Issue[]){setIssues(next);form.setFields(next.map(x=>({name:x.field==='basePrice'?'priceYuan':x.field.startsWith('galleryMediaIds')?'coverMediaId':x.field.split('.')[0],errors:[x.message]})))}
 function validate(publish:boolean):Issue[]{
  const v=form.getFieldsValue(true),out:Issue[]=[]
  for(const [field,label,max,required] of [['name','服务名称',60,true],['unit','单位',8,true],['serviceDescription','服务说明',5000,true],['serviceNotice','服务须知',3000,false]] as const){const text=(v[field]||'').trim();if(publish&&required&&!text)out.push({field,message:`请填写${label}`});else if(Array.from(text).length>max)out.push({field,message:`${label}最多${max}字`})}
  if(publish&&!v.categoryId)out.push({field:'categoryId',message:'请选择服务分类'})
  const price=String(v.priceYuan||'').trim()
  if((publish&&!price)||(price&&(!/^\d+(\.\d{1,2})?$/.test(price)||Number(price)>999999.99||(publish&&Number(price)<=0))))out.push({field:'basePrice',message:'价格需为0.01～999999.99元，最多两位小数'})
  if(publish&&!files.some(f=>f.status==='done'&&f.response?.id))out.push({field:'coverMediaId',message:'请上传服务主图'})
  if(publish&&files.some(f=>f.status==='uploading'||f.status==='error'))out.push({field:'coverMediaId',message:'请等待图片上传完成，失败图片请重试或移除'})
  return out
 }
 async function save(publish:boolean,confirmed=false){
  if(busy)return
  form.setFields(Object.keys(form.getFieldsValue(true)).map(name=>({name,errors:[]})))
  const found=validate(publish);showIssues(found);if(found.length)return
  const v=form.getFieldsValue(true),basePrice=Math.round(Number(v.priceYuan||0)*100)
  if(publish&&detail.data?.publishedVersion&&!confirmed){
   if(!published.data){message.error('暂时无法读取当前已发布内容，请重试');return}
   const old=published.data,changes=[]
   if(old.price!==basePrice)changes.push('价格');if(old.unit!==(v.unit||'').trim())changes.push('单位');if(old.serviceDescription!==(v.serviceDescription||'').trim())changes.push('服务说明');if(old.serviceNotice!==(v.serviceNotice||'').trim())changes.push('服务须知')
   if(changes.length){modal.confirm({title:'确认发布更新',content:`本次将更新：${changes.join('、')}。已成交订单仍保留原内容。`,okText:'发布更新',onOk:()=>save(true,true)});return}
  }
  setBusy(true);let draftSaved=false
  try{
   const ids=files.filter(f=>f.status==='done'&&f.response?.id).map(f=>String(f.response.id))
   const body:SkuWrite={name:(v.name||'').trim(),categoryId:v.categoryId||'',basePrice,unit:(v.unit||'').trim(),serviceDescription:(v.serviceDescription||'').trim(),serviceNotice:(v.serviceNotice||'').trim(),priceMode:'FIXED',coverMediaId:ids[0]||'',galleryMediaIds:ids.slice(1),version:saved.current?.version??0}
   const result=saved.current?await updateSku(saved.current.id,body):await createSku(body)
   saved.current={id:result.id,version:result.version};draftSaved=true
   if(publish)await publishSku(result.id,result.version)
   dirtyRef.current=false;setDirty(false);message.success(publish?'服务已发布':files.some(f=>f.status!=='done')?'草稿已保存，仅保留上传成功的图片':'草稿已保存');navigate('/catalog/skus')
  }catch(e){if(e instanceof ApiError&&e.fieldErrors)showIssues(e.fieldErrors);message.error(`${draftSaved?'草稿已保存，发布失败：':''}${e instanceof Error?e.message:'保存失败，请重试'}`)}finally{setBusy(false)}
 }
 function move(index:number,delta:number){const next=[...files];[next[index],next[index+delta]]=[next[index+delta],next[index]];setFiles(next);change()}
 const issueLabel=(field:string)=>({name:'服务名称',categoryId:'服务分类',basePrice:'价格',unit:'单位',coverMediaId:'服务图片',galleryMediaIds:'服务图片',serviceDescription:'服务说明',serviceNotice:'服务须知'}[field.split('.')[0]]||'服务图片')
 const cover=files.find(f=>f.status==='done'&&f.response?.id)
 return <><Space style={{marginBottom:16}}><Typography.Title level={3} style={{margin:0}}>{id?'编辑服务':'新增服务'}</Typography.Title><Button className="sku-preview-toggle" onClick={()=>setPreview(!preview)}>{preview?'继续编辑':'预览'}</Button></Space>
 {detail.error&&<Alert type="error" message="服务加载失败，请刷新重试"/>}
 {issues.length>0&&<Alert style={{marginBottom:16}} type="error" message={`还有 ${issues.length} 项需要处理`} description={<div className="sku-error-links">{issues.map((x,i)=><Button type="link" key={i} onClick={()=>{setPreview(false);setTimeout(()=>{const field=x.field==='basePrice'?'priceYuan':x.field.split('.')[0];if(field==='coverMediaId'||field==='galleryMediaIds')document.getElementById('sku-images')?.scrollIntoView({block:'center'});else form.scrollToField(field,{block:'center'})},0)}}>{issueLabel(x.field)}：{x.message}</Button>)}</div>}/>}
 <div className={`sku-editor ${preview?'preview-mode':''}`}><Card className="sku-form-card" loading={detail.isLoading}><Form form={form} layout="vertical" onValuesChange={change} disabled={busy}>
 <Form.Item label="服务名称" name="name" required><Input placeholder="例如：墙面局部补漆"/></Form.Item>
 <Form.Item label="服务分类" name="categoryId" required><Select loading={categories.isLoading} placeholder="请选择分类" options={categories.data?.filter(c=>c.status==='ACTIVE').map(c=>({label:c.name,value:c.id}))}/></Form.Item>
 <Space align="start" wrap><Form.Item label="价格（元）" name="priceYuan" required><Input inputMode="decimal" placeholder="例如：100"/></Form.Item><Form.Item label="单位" name="unit" required><AutoComplete style={{width:160}} options={['次','处','台','小时','㎡','份'].map(value=>({value}))} placeholder="选择或输入单位"/></Form.Item></Space>
 <div id="sku-images"><Form.Item label="服务图片" name="coverMediaId" required extra="第一张为主图，一张即可发布；最多6张，无需填写图片说明。">
 <div><SkuImageUpload listType="picture-card" maxCount={6} fileList={files} disabled={busy} onChange={({fileList})=>{setFiles(fileList);change()}} customRequest={async({file,onSuccess,onError})=>{try{const result=await uploadFile('/api/v1/admin/media/images',file as File);onSuccess?.(result)}catch(e){onError?.(e as Error);message.error('图片上传失败，请重试或移除')}}}>{files.length<6?'上传图片':null}</SkuImageUpload>{files.length>1&&files.map((f,i)=><div className="sku-image-order" key={f.uid}><span>{i===0?'主图':`附图${i}`} · {f.name}</span><Button size="small" disabled={i===0||busy} onClick={()=>move(i,-1)}>前移</Button><Button size="small" disabled={i===files.length-1||busy} onClick={()=>move(i,1)}>后移</Button></div>)}</div>
 </Form.Item></div>
 <Form.Item label="服务说明" name="serviceDescription" required extra="写清提供什么服务，以及一份服务包含多大范围。支持换行，无需按固定格式填写。"><Input.TextArea autoSize={{minRows:5,maxRows:14}} showCount count={{strategy:value=>Array.from(value.trim()).length,max:5000}} placeholder="例如：提供室内墙面局部补漆，包含保护、表面处理、涂刷和清理。每处面积不超过……"/></Form.Item>
 <Form.Item label="服务须知（选填）" name="serviceNotice" extra="可填写材料责任、准备事项、额外费用及不适用情况；没有可留空。"><Input.TextArea autoSize={{minRows:3,maxRows:10}} showCount count={{strategy:value=>Array.from(value.trim()).length,max:3000}}/></Form.Item>
 <div className="sku-actions"><Space><Button onClick={()=>navigate('/catalog/skus')} disabled={busy}>取消</Button><Button loading={busy} onClick={()=>save(false)}>保存草稿</Button><Button type="primary" loading={busy} onClick={()=>save(true)}>{detail.data?.publishedVersion?'发布更新':'发布服务'}</Button>{dirty&&<Typography.Text type="secondary">有未保存修改</Typography.Text>}</Space></div>
 </Form></Card><aside className="sku-preview"><small>当前编辑预览 · 尚未发布</small>{cover&&<img className="hero" src={`/api/v1/admin/media/${cover.response.id}/content`} alt="服务主图"/>}<h2>{values.name||'服务名称'}</h2><div className="price">¥{values.priceYuan||'—'} / {values.unit||'单位'}</div><ServiceContent serviceDescription={values.serviceDescription} serviceNotice={values.serviceNotice}/></aside></div></>
}
