import PluginIcon from '../../../packages/ui-components/PluginIcon';
import PluginDetails from './PluginDetails';
import type {ReactNode} from 'react';
import type {ManagedPlugin} from './PluginManager';
import { useEffect, useRef, useState } from 'react';

type Asset = {os:string;arch:string;file:string;sha256:string;size:number;assetId:number;downloadUrl:string};
type Release = {version:string;repository:string;minHostVersion:string;compatibility:{apiVersion:string;uiBridgeVersion:string};assets:Asset[]};
type Entry = {icon?:string;id:string;name:string;summary:string;publisher:{kind:string};source:{repository:string|null;path:string};status:string;releases:Release[];installedVersion:string;development:boolean;changed:boolean;protected:boolean};
export type StoreSnapshot = {entries:Entry[];revision:string;updatedAt:number;stale:boolean;notice:string;os:string;arch:string;hostVersion:string};
export type StorePreview = {ticket:string;id:string;name:string;version:string;kind:string;installedVersion:string;changed:boolean;repository:string;archiveSha256:string;expiresAt:number};
export type StoreBridge = {load(refresh:boolean):Promise<StoreSnapshot>;prepare(id:string,version:string,revision:string):Promise<StorePreview>;apply(ticket:string,trust:boolean):Promise<unknown>;cancel(ticket:string):Promise<void>};
type FilterKind = 'all' | 'official' | 'community' | 'installed' | 'updatable';

const PAGE_SIZE = 24;

function compare(a:string,b:string){const aa=a.split('.').map(Number),bb=b.split('.').map(Number);for(let i=0;i<3;i++){if(aa[i]!==bb[i])return aa[i]>bb[i]?1:-1;}return 0;}
function compatible(release:Release,data:StoreSnapshot){return release.compatibility.apiVersion==='1'&&['1','2'].includes(release.compatibility.uiBridgeVersion)&&compare(data.hostVersion,release.minHostVersion)>=0&&release.assets.some(asset=>asset.os===data.os&&asset.arch===data.arch);}

export default function PluginStore({bridge,onChanged,initialID='',renderActions,onBusyChange,plugins,disabled}:{bridge:StoreBridge;onChanged():Promise<void>;initialID?:string;renderActions(id:string):ReactNode;onBusyChange(busy:boolean):void;plugins:ManagedPlugin[];disabled:boolean}){
  const [data,setData]=useState<StoreSnapshot>();
  const [query,setQuery]=useState('');
  const [filter,setFilter]=useState<FilterKind>('all');
  const [page,setPage]=useState(1);
  const [selectedID,setSelectedID]=useState(initialID);
  const [version,setVersion]=useState('');
  const [localBusy,setBusy]=useState(false);
  const [processingID,setProcessingID]=useState('');
  const busy=localBusy||disabled;
  const [error,setError]=useState('');
  const [notice,setNotice]=useState('');
  const [preview,setPreview]=useState<StorePreview>();
  const alive=useRef(true),pending=useRef(false),ticket=useRef('');
  function clearPreview(){const previous=ticket.current;ticket.current='';setPreview(undefined);if(previous)void bridge.cancel(previous).catch(()=>{});}
  async function perform(action:()=>Promise<void>){
    if(pending.current||disabled)return;pending.current=true;setBusy(true);setError('');setNotice('');
    try{await action();}catch(reason){if(alive.current)setError(reason instanceof Error?reason.message:'操作失败，请重试。');}
    finally{pending.current=false;if(alive.current)setBusy(false);}
  }
  async function load(refresh:boolean){const next=await bridge.load(refresh);if(alive.current)setData(next);}
  useEffect(()=>{
    alive.current=true;void perform(()=>load(false));
    return()=>{alive.current=false;if(ticket.current)void bridge.cancel(ticket.current).catch(()=>{});};
  },[bridge]);
  useEffect(()=>{setSelectedID(initialID);setVersion('');},[initialID]);
  useEffect(()=>{clearPreview();},[selectedID,version,data?.revision,plugins]);
  useEffect(()=>{onBusyChange(localBusy);return()=>onBusyChange(false);},[localBusy,onBusyChange]);

  const allEntries=(data?.entries??[]).map(entry=>{
    const local=plugins.find(plugin=>plugin.id===entry.id);
    const installedVersion=local?.version??'';
    const latestCompatible=data?entry.releases.find(item=>compatible(item,data)):undefined;
    const latestRelease=latestCompatible??entry.releases[0];
    const latestVersion=latestRelease?.version??'';
    const updatable=Boolean(entry.status==='published'&&installedVersion&&latestCompatible&&compare(latestCompatible.version,installedVersion)>0&&!local?.development&&!local?.changed&&!local?.protected);
    return {...entry,installedVersion,development:!!local?.development,changed:!!local?.changed,protected:!!local?.protected,latestVersion,latestCompatible,updatable};
  });

  const searchedEntries=allEntries.filter(entry=>`${entry.name} ${entry.id} ${entry.summary}`.toLowerCase().includes(query.trim().toLowerCase()));
  const counts: Record<FilterKind, number> = {
    all: searchedEntries.length,
    official: searchedEntries.filter(e=>e.publisher.kind==='official').length,
    community: searchedEntries.filter(e=>e.publisher.kind!=='official').length,
    installed: searchedEntries.filter(e=>!!e.installedVersion).length,
    updatable: searchedEntries.filter(e=>e.updatable).length,
  };
  const entries=searchedEntries.filter(entry=>{
    if(filter==='official')return entry.publisher.kind==='official';
    if(filter==='community')return entry.publisher.kind!=='official';
    if(filter==='installed')return !!entry.installedVersion;
    if(filter==='updatable')return entry.updatable;
    return true;
  });

  const totalPages=Math.max(1,Math.ceil(entries.length/PAGE_SIZE));
  const currentPage=Math.min(page,totalPages);
  const pagedEntries=entries.slice((currentPage-1)*PAGE_SIZE,currentPage*PAGE_SIZE);

  const selected=selectedID?allEntries.find(entry=>entry.id===selectedID):undefined;
  const releases=selected?.releases??[];
  const release=releases.find(item=>item.version===version)??releases.find(item=>data&&compatible(item,data))??releases[0];
  function blockedFor(entry:Entry, target?:Release) {
    return entry.development?'开发链接请使用重载':entry.changed?'请先处理本地文件修改':entry.protected?'随宿主更新':data?.stale?'请联网刷新后安装':entry.status==='withdrawn'?'已下架':entry.status!=='published'?'待发布':!target||!data||!compatible(target,data)?'当前平台或宿主不兼容':'';
  }
  const blocked=selected?blockedFor(selected,release):'';

  async function applyPrepared(candidate:StorePreview, trustNative=false) {
    // Clear the UI's cancellation handle before consuming the one-use ticket.
    // Navigation must not cancel an already authorized apply operation.
    ticket.current='';
    if(alive.current){setPreview(undefined);setProcessingID(candidate.id);}
    try {
      await bridge.apply(candidate.ticket,trustNative);
      await onChanged();
      if(alive.current){setNotice(candidate.installedVersion?'插件已更新。':'插件已安装。');await load(false);}
    } finally {
      await bridge.cancel(candidate.ticket).catch(()=>{});
      if(alive.current)setProcessingID('');
    }
  }

  function install(entry:Entry, target?:Release) {
    if(busy||!data||!target||blockedFor(entry,target))return;
    clearPreview();
    void perform(async()=>{
      setProcessingID(entry.id);
      try {
        const result=await bridge.prepare(entry.id,target.version,data.revision);
        if(!alive.current){await bridge.cancel(result.ticket);return;}
        ticket.current=result.ticket;
        if(!result.changed){clearPreview();setNotice('插件已是当前内容。');return;}
        if(result.kind==='process'){setPreview(result);return;}
        await applyPrepared(result);
      } finally {if(alive.current)setProcessingID('');}
    });
  }
  const asset=release?.assets.find(item=>item.os===data?.os&&item.arch===data?.arch);

  const filterOptions: [FilterKind, string][] = [
    ['all', '全部'],
    ['official', '官方'],
    ['community', '社区'],
    ['installed', '已安装'],
    ['updatable', '可更新'],
  ];

  const previewPanel = preview&&<div className="store-preview" role="status">
    <strong>{preview.installedVersion?`${preview.installedVersion} → ${preview.version}`:preview.version}</strong>
    <p className="store-note">此原生插件将以当前用户权限运行。</p>
    <div className="store-actions"><button disabled={busy} onClick={clearPreview}>取消</button><button className="primary" disabled={busy} onClick={()=>void perform(()=>applyPrepared(preview,true))}>{preview.installedVersion?'信任并更新':'信任并安装'}</button></div>
  </div>;

  return <div className="store-body">
    {selectedID ? <div className="store-detail-view">
      <div className="store-detail-nav">
        <button type="button" className="store-back" disabled={busy} onClick={()=>{clearPreview();setSelectedID('');setVersion('');}}>← 返回插件目录</button>
        <button type="button" disabled={busy} onClick={()=>{clearPreview();void perform(()=>load(true));}}>刷新目录</button>
      </div>
      {data?.notice&&<p className="store-note" role="status">{data.notice}</p>}
      {!selected&&data&&!busy&&<div className="store-detail"><p className="store-note">此插件未收录于商店，本地管理仍可使用。</p></div>}
      {selected&&<PluginDetails plugin={selected}>
        <div className="store-meta-badges">
          <span className={`store-publisher ${selected.publisher.kind==='official'?'official':'community'}`}>{selected.publisher.kind==='official'?'官方':'社区'}</span>
          {selected.installedVersion&&<span className="store-badge installed">本地版本 {selected.installedVersion}</span>}
          {asset&&data&&<span className="store-badge version">{data.os} / {data.arch} · {(asset.size/1024/1024).toFixed(1)} MiB</span>}
        </div>
        <p className="store-summary">{selected.summary}</p>
        <p className="store-source">{selected.source.repository?.replace('https://github.com/','')??'源码地址待登记'}</p>
        {releases.length>0&&<label className="store-version">版本<select value={release?.version??''} disabled={busy} onChange={event=>setVersion(event.target.value)}>{releases.map(item=><option value={item.version} key={item.version}>{item.version}</option>)}</select></label>}
        {blocked?<p className="store-note">{blocked}</p>:!preview&&<button className="primary" disabled={busy} onClick={()=>install(selected,release)}>{processingID===selected.id?'处理中…':selected.installedVersion?(compare(release!.version,selected.installedVersion)<0?'安装此版本':'更新'):'安装'}</button>}
        {previewPanel}
        {selected.installedVersion&&renderActions(selected.id)}
      </PluginDetails>}
    </div> : <>
      <div className="store-toolbar">
        <input aria-label="搜索插件商店" placeholder="搜索插件名称、ID 或简介…" value={query} onChange={event=>{clearPreview();setQuery(event.target.value);setPage(1);setVersion('');}} disabled={busy}/>
        <button disabled={busy} onClick={()=>{clearPreview();void perform(()=>load(true));}}>刷新目录</button>
      </div>
      <div className="store-filters" role="group" aria-label="筛选插件目录">
        <div className="store-filter-pills">
          {filterOptions.map(([kind,label])=><button key={kind} type="button" className="store-filter-pill" aria-pressed={filter===kind} disabled={busy} onClick={()=>{clearPreview();setFilter(kind);setPage(1);}}>{label} <small>{counts[kind]}</small></button>)}
        </div>
        <span className="store-count">{entries.length} 个插件</span>
      </div>
      {data?.notice&&<p className="store-note" role="status">{data.notice}</p>}
      <div className="store-grid" aria-label="商店插件">
        {pagedEntries.map(entry=>{
          const reason=blockedFor(entry,entry.latestCompatible);
          const installed=!!entry.installedVersion&&!entry.updatable;
          const label=entry.status==='withdrawn'?'已下架':entry.status!=='published'?'待发布':installed?'已安装':!entry.latestCompatible?'不兼容':entry.updatable?'更新':'安装';
          return <article key={entry.id} className="store-card">
            <button type="button" className="store-card-open" disabled={busy} aria-label={`查看 ${entry.name} 详情`} onClick={()=>{clearPreview();setSelectedID(entry.id);setVersion('');}}>
              <PluginIcon name={entry.name} icon={entry.icon}/>
              <span className="store-item-label">
                <strong title={entry.name}>{entry.name}</strong>
                <small className="store-card-summary" title={entry.summary}>{entry.summary||'暂无简介'}</small>
              </span>
            </button>
            <div className="store-card-footer">
              <div className="store-card-meta">
                <span className={`store-card-publisher ${entry.publisher.kind==='official'?'official':'community'}`}>{entry.publisher.kind==='official'?'官方':'社区'}</span>
                {(entry.installedVersion||entry.latestVersion)&&<span className="store-card-version" title={entry.installedVersion?`本地版本 ${entry.installedVersion}${entry.updatable?`，可更新至 ${entry.latestVersion}`:''}`:`版本 ${entry.latestVersion}`}>v{entry.installedVersion||entry.latestVersion}</span>}
              </div>
              {preview?.id!==entry.id&&<div className="store-card-actions"><button type="button" className={installed||reason?'':'primary'} disabled={busy||installed||!!reason} title={reason||(installed?'已是当前可用版本':'')} aria-label={`${label} ${entry.name}`} onClick={()=>install(entry,entry.latestCompatible)}>{processingID===entry.id?(entry.installedVersion?'更新中…':'安装中…'):label}</button></div>}
            </div>
            {preview?.id===entry.id&&previewPanel}
          </article>;
        })}
      </div>
      {!entries.length&&<p className="empty">{busy?'正在读取目录…':query||filter!=='all'?'没有匹配的插件':'暂无插件'}</p>}
      {totalPages>1&&<div className="store-pagination" role="navigation" aria-label="目录分页">
        <button type="button" disabled={busy||currentPage<=1} onClick={()=>{clearPreview();setPage(p=>Math.max(1,p-1));}}>上一页</button>
        <span>第 {currentPage} / {totalPages} 页</span>
        <button type="button" disabled={busy||currentPage>=totalPages} onClick={()=>{clearPreview();setPage(p=>Math.min(totalPages,p+1));}}>下一页</button>
      </div>}
    </>}
    {error&&<p className="error" role="alert">{error}</p>}{notice&&<p className="settings-notice" role="status">{notice}</p>}
  </div>;
}
