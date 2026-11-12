import { useEffect, useRef, useState } from 'react';

type Asset = {os:string;arch:string;file:string;sha256:string;size:number;assetId:number;downloadUrl:string};
type Release = {version:string;repository:string;minHostVersion:string;compatibility:{apiVersion:string;uiBridgeVersion:string};assets:Asset[]};
type Entry = {id:string;name:string;summary:string;publisher:{kind:string};source:{repository:string|null;path:string};status:string;releases:Release[];installedVersion:string;development:boolean;changed:boolean;protected:boolean};
export type StoreSnapshot = {entries:Entry[];revision:string;updatedAt:number;stale:boolean;notice:string;os:string;arch:string;hostVersion:string};
export type StorePreview = {ticket:string;id:string;name:string;version:string;kind:string;installedVersion:string;changed:boolean;repository:string;archiveSha256:string;expiresAt:number};
export type StoreBridge = {load(refresh:boolean):Promise<StoreSnapshot>;prepare(id:string,version:string,revision:string):Promise<StorePreview>;apply(ticket:string,trust:boolean):Promise<unknown>;cancel(ticket:string):Promise<void>};
function compare(a:string,b:string){const aa=a.split('.').map(Number),bb=b.split('.').map(Number);for(let i=0;i<3;i++){if(aa[i]!==bb[i])return aa[i]>bb[i]?1:-1;}return 0;}
function compatible(release:Release,data:StoreSnapshot){return release.compatibility.apiVersion==='1'&&['1','2'].includes(release.compatibility.uiBridgeVersion)&&compare(data.hostVersion,release.minHostVersion)>=0&&release.assets.some(asset=>asset.os===data.os&&asset.arch===data.arch);}

export default function PluginStore({bridge,onChanged}:{bridge:StoreBridge;onChanged():Promise<void>}){
  const [data,setData]=useState<StoreSnapshot>();
  const [query,setQuery]=useState('');
  const [selectedID,setSelectedID]=useState('');
  const [version,setVersion]=useState('');
  const [busy,setBusy]=useState(false);
  const [error,setError]=useState('');
  const [notice,setNotice]=useState('');
  const [preview,setPreview]=useState<StorePreview>();
  const [trust,setTrust]=useState(false);
  const alive=useRef(true),pending=useRef(false),ticket=useRef('');
  function clearPreview(){const previous=ticket.current;ticket.current='';setPreview(undefined);setTrust(false);if(previous)void bridge.cancel(previous).catch(()=>{});}
  async function perform(action:()=>Promise<void>){
    if(pending.current)return;pending.current=true;setBusy(true);setError('');setNotice('');
    try{await action();}catch(reason){if(alive.current)setError(reason instanceof Error?reason.message:'操作失败，请重试。');}
    finally{pending.current=false;if(alive.current)setBusy(false);}
  }
  async function load(refresh:boolean){const next=await bridge.load(refresh);if(alive.current)setData(next);}
  useEffect(()=>{
    alive.current=true;void perform(()=>load(false));
    return()=>{alive.current=false;if(ticket.current)void bridge.cancel(ticket.current).catch(()=>{});};
  },[bridge]);
  useEffect(()=>{clearPreview();},[selectedID,version,data?.revision]);
  const entries=(data?.entries??[]).filter(entry=>`${entry.name} ${entry.id} ${entry.summary}`.toLowerCase().includes(query.trim().toLowerCase()));
  const selected=entries.find(entry=>entry.id===selectedID)??entries[0];
  const releases=selected?.releases??[];
  const release=releases.find(item=>item.version===version)??releases.find(item=>data&&compatible(item,data))??releases[0];
  const blocked=selected?.development?'开发链接请在插件管理中重载':selected?.changed?'请先处理本地文件修改':selected?.protected?'随宿主更新':data?.stale?'请联网刷新后安装':selected?.status==='withdrawn'?'已下架':selected?.status!=='published'?'待发布':!release||!data||!compatible(release,data)?'当前平台或宿主不兼容':'';
  const asset=release?.assets.find(item=>item.os===data?.os&&item.arch===data?.arch);
  return <div className="store-body">
    <div className="store-toolbar"><input aria-label="搜索插件商店" placeholder="搜索插件…" value={query} onChange={event=>{clearPreview();setQuery(event.target.value);setVersion('');}} disabled={busy}/><button disabled={busy} onClick={()=>{clearPreview();void perform(()=>load(true));}}>刷新目录</button></div>
    {data?.notice&&<p className="store-note" role="status">{data.notice}</p>}
    <div className="store-layout">
      <div className="store-list" aria-label="商店插件">
        {entries.map(entry=><button key={entry.id} className={`store-item${selected?.id===entry.id?' selected':''}`} disabled={busy} onClick={()=>{setSelectedID(entry.id);setVersion('');}}><strong>{entry.name}</strong><small>{entry.publisher.kind==='official'?'官方':'社区'}{entry.installedVersion?` · 已安装 ${entry.installedVersion}`:entry.status==='pending-release'?' · 待发布':''}</small></button>)}
        {!entries.length&&<p className="empty">{busy?'正在读取目录…':query?'没有匹配的插件':'暂无插件'}</p>}
      </div>
      {selected&&<section className="store-detail" aria-label={`${selected.name} 详情`}>
        <h2>{selected.name}</h2><small>{selected.id}</small><p className="store-summary">{selected.summary}</p>
        <p className="store-source">{selected.source.repository?.replace('https://github.com/','')??'源码地址待登记'}</p>
        {releases.length>0&&<label className="store-version">版本<select value={release?.version??''} disabled={busy} onChange={event=>setVersion(event.target.value)}>{releases.map(item=><option value={item.version} key={item.version}>{item.version}</option>)}</select></label>}
        {selected.installedVersion&&<p className="store-note">本地版本 {selected.installedVersion}</p>}
        {asset&&data&&<p className="store-note">{data.os} / {data.arch} · {(asset.size/1024/1024).toFixed(1)} MiB</p>}
        {blocked?<p className="store-note">{blocked}</p>:!preview&&<button className="primary" disabled={busy} onClick={()=>void perform(async()=>{
          const result=await bridge.prepare(selected.id,release!.version,data!.revision);
          if(!alive.current){await bridge.cancel(result.ticket);return;}
          ticket.current=result.ticket;setPreview(result);setTrust(false);
        })}>{busy?'正在检查…':selected.installedVersion?(compare(release!.version,selected.installedVersion)<0?'检查旧版本':'检查更新'):'检查安装'}</button>}
        {preview&&<div className="store-preview" role="status">
          <strong>{preview.installedVersion?`${preview.installedVersion} → ${preview.version}`:preview.version}</strong>
          <p className="store-note">{preview.repository}</p>
          {preview.kind==='process'&&preview.changed&&<label className="trust"><input type="checkbox" checked={trust} disabled={busy} onChange={event=>setTrust(event.target.checked)}/>信任此原生插件，允许其以当前用户权限运行</label>}
          <div className="store-actions"><button disabled={busy} onClick={clearPreview}>取消</button><button className="primary" disabled={busy||!preview.changed||preview.kind==='process'&&!trust} onClick={()=>void perform(async()=>{
            const current=ticket.current;
            try{await bridge.apply(current,trust);await onChanged();if(alive.current){setNotice('插件已安装。');await load(false);}}
            finally{ticket.current='';if(alive.current){setPreview(undefined);setTrust(false);}}
          })}>{!preview.changed?'已是当前内容':preview.installedVersion?(compare(preview.version,preview.installedVersion)<0?'确认降级':'确认更新'):'确认安装'}</button></div>
        </div>}
      </section>}
    </div>
    {error&&<p className="error" role="alert">{error}</p>}{notice&&<p className="settings-notice" role="status">{notice}</p>}
  </div>;
}
