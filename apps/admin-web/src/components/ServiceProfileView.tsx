import { useEffect, useState } from 'react';
import type { ServiceProfile } from '../api/serviceProfile';
import './ServiceProfileView.css';
const media = (id?: string) => id ? `/api/v1/admin/media/${id}/content` : '';
function ServicePhoto({id,alt}:{id?:string;alt:string}){const[failed,setFailed]=useState(false);useEffect(()=>setFailed(false),[id]);return id&&!failed?<img src={media(id)} alt={alt} onError={()=>setFailed(true)}/>:<div className="sp-photo-unavailable">图片暂时无法显示</div>}
export function ServiceProfileView({ profile: p }: {
    profile?: Partial<ServiceProfile>;
}) {
    if (!p)
        return null;
    const h = p.enhancements, a = p.afterSales;
    return <div className="service-profile">
  {!!h?.tags?.length && <div className="sp-tags">{h.tags.map((t, i) => <span key={i}>{t}</span>)}</div>}
  <section><h3>本服务包含</h3><div className="sp-items">{p.includedItems?.map((v, i) => <article key={i}>{v.imageMediaId ? <ServicePhoto id={v.imageMediaId} alt={v.title}/> : <span className="sp-icon"><img className="sp-icon-svg" src={`/service-icons/${v.iconKey || 'service'}.svg`} alt=""/></span>}<div><strong>{v.title}</strong><p>{v.description}</p></div></article>)}</div></section>
  <section className="sp-notice"><h3>费用与服务边界</h3><p>{p.serviceTarget}</p><p>{p.billing?.unitDefinition}</p><p>{p.billing?.quantityExplanation}</p><p>{p.billing?.feeDescription}</p>{p.excludedItems?.map((v, i) => <p key={i}><strong>不包含 · {v.title}</strong> {v.description}</p>)}{p.unsuitableConditions?.map((v, i) => <p key={i}>不适用：{v}</p>)}{p.purchaseNotices?.map((v, i) => <p key={i}>{v}</p>)}</section>
  {!!h?.facts?.length && <section><h3>服务参数</h3><div className="sp-facts">{h.facts.map((v, i) => <div key={i}><small>{v.label}</small><p>{v.value}</p></div>)}</div></section>}
  {!!h?.resources?.length && <section><h3>工具与材料</h3>{h.resources.map((v, i) => <p key={i}><strong>{v.name} · {{ PROVIDER: '服务方提供', CUSTOMER: '客户提供', SHARED: '双方准备' }[v.provider]}</strong><br />{v.description}</p>)}</section>}
  {!!h?.preparations?.length && <section><h3>服务前准备</h3>{h.preparations.map((v, i) => <p key={i}>✓ {v}</p>)}</section>}
  {!!h?.steps?.length && <section><h3>服务流程</h3>{h.steps.map((v, i) => <article className="sp-step" key={i}><span>{i + 1}</span><div><strong>{v.title}</strong><p>{v.description}</p></div></article>)}</section>}
  {!!h?.gallery?.length && <section><h3>服务案例</h3>{h.gallery.map((v, i) => <figure key={i}><ServicePhoto id={v.mediaId} alt={v.caption}/><figcaption>{v.caption}</figcaption></figure>)}</section>}
  {!!h?.comparisons?.length && <section><h3>前后对照</h3>{h.comparisons.map((v, i) => <div key={i}><strong>{v.title}</strong><div className="sp-facts"><figure><ServicePhoto id={v.beforeMediaId} alt="服务前"/><figcaption>服务前</figcaption></figure><figure><ServicePhoto id={v.afterMediaId} alt="服务后"/><figcaption>服务后</figcaption></figure></div><p>{v.description}</p><small>{v.effectNotice}</small></div>)}</section>}
  <section><h3>交付检查</h3>{p.acceptanceCriteria?.map((v, i) => <p key={i}>✓ {v}</p>)}</section>
  {a && <section className="sp-policy"><h3>{{ WARRANTY: '质保说明', REWORK: '复洁或重做说明', SUPPORT: '售后支持' }[a.type]}</h3><strong>{a.summary}</strong>{a.period && <p>{a.period.value}{{ HOUR: '小时', DAY: '天', MONTH: '个月' }[a.period.unit]} · {a.period.startDescription}</p>}<p>{a.coverage}</p>{a.exclusions?.map((v, i) => <p key={i}>不包含：{v}</p>)}<p>{a.contactDescription}</p></section>}
  {!!h?.faqs?.length && <section><h3>常见问题</h3>{h.faqs.map((v, i) => <details key={i}><summary>{v.question}</summary><p>{v.answer}</p></details>)}</section>}
 </div>;
}
