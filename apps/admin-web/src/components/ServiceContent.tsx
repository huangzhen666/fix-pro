import './ServiceContent.css'
export function ServiceContent({serviceDescription,serviceNotice}:{serviceDescription?:string;serviceNotice?:string}) {
 return <div className="service-content">{serviceDescription?.trim()&&<section><h3>服务说明</h3><p>{serviceDescription}</p></section>}{serviceNotice?.trim()&&<section className="service-notice"><h3>服务须知</h3><p>{serviceNotice}</p></section>}</div>
}
