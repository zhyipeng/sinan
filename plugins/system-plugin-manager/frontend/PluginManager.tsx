import PluginIcon from '../../../packages/ui-components/PluginIcon';
import PluginStore, {type StoreBridge,type InstallTask} from './PluginStore';
import PluginDetails from './PluginDetails';
import { useEffect, useRef, useState } from 'react';
import CreatePlugin, {type ProjectBridge} from './CreatePlugin';
import PluginData,{type DataBridge} from './PluginData';

export type ManagedPlugin = {dependencies?:{id:string;pluginId:string;state:string;message:string;optional:boolean;minVersion:string}[]|null;requiredBy?:string[]|null;icon?: string;development?: boolean; developmentFingerprint?: string; needsTrust?: boolean; protected?: boolean; id: string; name: string; version: string; kind: string; state: string; changed?: boolean; disabled?: boolean; uiViews?: {id:string;title?:string}[] | null};
export type ManagedSnapshot = {plugins: ManagedPlugin[]; diagnostics: string[]; uiEnabled?: boolean};
export type PackagePreview = {id: string; name: string; version: string; kind: string; installedVersion: string; fingerprint: string; installedFingerprint: string; changed: boolean};
export type ManagementBridge = DataBridge & ProjectBridge & {
  chooseInstallDirectory(): Promise<string>;
  chooseInstallArchive(): Promise<string>;
  trustDevelopment(id:string,fingerprint:string):Promise<unknown>;
  refresh(): Promise<ManagedSnapshot>;
  stop(id: string): Promise<void>;
  uninstall(id: string): Promise<void>;
  reload(id: string): Promise<unknown>;
  inspect(directory: string): Promise<PackagePreview>;
  apply(directory: string, trust: boolean, fingerprint: string, installedFingerprint: string): Promise<unknown>;
  setEnabled(id:string,enabled:boolean):Promise<unknown>;
  openView?(id:string,viewID:string):Promise<void>;
  subscribeWindows?(failed:()=>void):()=>void;
};
function downloadSize(bytes:number){return bytes<1024*1024?`${(bytes/1024).toFixed(1)} KiB`:`${(bytes/1024/1024).toFixed(1)} MiB`;}
const states: Record<string,string> = {installed:'已安装',activating:'启动中',activated:'已激活',running:'运行中',deactivated:'已停止',failed:'运行失败',disabled:'已禁用',blocked:'依赖不可用'};

export default function PluginManager({bridge, onChanged, onRecover,storeBridge}: {bridge: ManagementBridge; storeBridge: StoreBridge; onChanged(): Promise<void>; onRecover(): void}) {
  const [data,setData]=useState<ManagedSnapshot>({plugins:[],diagnostics:[]});
  const [busy,setBusy]=useState(true);
  const [showCreate,setShowCreate]=useState(false);
  const [tab,setTab]=useState<'installed'|'discover'|'downloads'>('installed');
  const [query,setQuery]=useState('');
  const [selectedID,setSelectedID]=useState('');
  const [storeID,setStoreID]=useState('');
  const [showInstall,setShowInstall]=useState(false);
  const [showMore,setShowMore]=useState(false);
  const [createBusy,setCreateBusy]=useState(false);
  const [storeBusy,setStoreBusy]=useState(false);
  const [tasks,setTasks]=useState<InstallTask[]>([]);
  const changed=useRef(onChanged);changed.current=onChanged;
  const projectBusy=createBusy||storeBusy;
  const [directory,setDirectory]=useState('');
  const [developmentTrust,setDevelopmentTrust]=useState<Record<string,boolean>>({});
  const [error,setError]=useState('');
  const [notice,setNotice]=useState('');
  const alive=useRef(false);
  const pending=useRef(false);
  useEffect(()=>{
    let current=true; alive.current=true;
    bridge.refresh().then(next=>{if(current)setData(next);})
      .catch(()=>{if(current)setError('无法读取插件，请刷新重试。');})
      .finally(()=>{if(current)setBusy(false);});
    return ()=>{current=false;alive.current=false;};
  },[bridge]);

  useEffect(()=>bridge.subscribeWindows?.(()=>setError('插件窗口异常退出，请重试。')),[bridge]);

  useEffect(()=>{
    let current=true;
    let timer:ReturnType<typeof setTimeout>;
    let completed='';
    async function poll(){
      try {
        const next=await storeBridge.tasks();
        if(!current)return;
        setTasks(next);
        const signature=next.filter(task=>task.state==='succeeded').map(task=>task.id).join(',');
        if(signature!==completed){
          const snapshot=await bridge.refresh();
          await changed.current();
          if(current){setData(snapshot);completed=signature;}
        }
      } catch(reason){if(current)setError(reason instanceof Error?reason.message:'无法读取安装任务');}
      finally {if(current)timer=setTimeout(()=>void poll(),1000);}
    }
    void poll();
    return()=>{current=false;clearTimeout(timer);};
  },[bridge,storeBridge]);

  async function perform(action:()=>Promise<unknown>, message:string|(()=>string)='') {
    if(pending.current||projectBusy)return;
    pending.current=true;setBusy(true);setError('');setNotice('');
    try {
      await action();
      const next=await bridge.refresh();
      // Keep the Shell's command registry current even if this view was closed.
      await onChanged();
      if(alive.current){setData(next);setNotice(typeof message==='function'?message():message);}
    } catch(error) {if(alive.current)setError(error instanceof Error?error.message:'操作失败，请重试。');}
    finally {pending.current=false;if(alive.current)setBusy(false);}
  }
  function installPackage() {
    if(busy||projectBusy||!directory.trim())return;
    const source=directory;
    let message='';
    void perform(async()=>{
      const candidate=await bridge.inspect(source);
      if(!alive.current)return;
      if(!candidate.changed){message='插件已是当前内容。';return;}
      await bridge.apply(source,candidate.kind==='process',candidate.fingerprint,candidate.installedFingerprint);
      message=candidate.installedVersion?'插件已更新。':'插件已安装。';
      if(alive.current)setDirectory('');
    },()=>message);
  }
  function changeDirectory(value: string) {
    setDirectory(value);setError('');setNotice('');
  }
  async function chooseInstallSource(archive = false) {
    if(pending.current||busy||projectBusy)return;
    pending.current=true;setBusy(true);
    try {
      const selected=await (archive?bridge.chooseInstallArchive():bridge.chooseInstallDirectory());
      if(selected&&alive.current)changeDirectory(selected);
    } catch(reason) {
      if(alive.current)setError(reason instanceof Error?reason.message:'无法选择插件包，请重试。');
    } finally {pending.current=false;if(alive.current)setBusy(false);}
  }
  const plugins=data.plugins.filter(plugin=>`${plugin.name} ${plugin.id}`.toLowerCase().includes(query.trim().toLowerCase()));
  const selected=plugins.find(plugin=>plugin.id===selectedID)??plugins[0];
  function renderActions(id:string) {
    const plugin=data.plugins.find(item=>item.id===id);
    if(!plugin)return null;
    return <div className="plugin-actions center-plugin-actions">
          {plugin.needsTrust&&<><label className="trust"><input type="checkbox" disabled={busy||projectBusy} checked={!!developmentTrust[plugin.id]} onChange={event=>setDevelopmentTrust(previous=>({...previous,[plugin.id]:event.target.checked}))}/>允许以当前用户权限运行</label><button disabled={busy||projectBusy||!developmentTrust[plugin.id]||plugin.changed} onClick={()=>void perform(async()=>{await bridge.trustDevelopment(plugin.id,plugin.developmentFingerprint!);setDevelopmentTrust(previous=>({...previous,[plugin.id]:false}));})}>信任开发插件</button></>}

          <button disabled={busy||projectBusy||!['running','activated','failed'].includes(plugin.state)} aria-label={`停止 ${plugin.name}`} onClick={()=>void perform(()=>bridge.stop(plugin.id))}>停止</button>
          {!plugin.protected&&<button className="danger" disabled={busy||projectBusy} aria-label={`卸载 ${plugin.name}`} onClick={()=>void perform(()=>bridge.uninstall(plugin.id))}>卸载</button>}
          <button disabled={busy||projectBusy} aria-label={`重载 ${plugin.name}`} onClick={()=>void perform(()=>bridge.reload(plugin.id))}>重载</button>
          {!plugin.protected&&<button disabled={busy||projectBusy} aria-label={`${plugin.disabled?'启用':'禁用'} ${plugin.name}`} onClick={()=>void perform(()=>bridge.setEnabled(plugin.id,!!plugin.disabled))}>{plugin.disabled?'启用':'禁用'}</button>}
          {data.uiEnabled&&plugin.uiViews?.map(view=><button key={view.id} disabled={busy||projectBusy||plugin.disabled||plugin.changed} aria-label={`打开 ${plugin.name} ${view.title||view.id}`} onClick={()=>void perform(()=>bridge.openView!(plugin.id,view.id))}>打开 {view.title||view.id}</button>)}
        </div>;
  }
  return <div className="management management-body plugin-center">
    <div className="center-navigation">
      <div className="center-tabs" role="tablist" aria-label="插件中心">
        <button id="installed-tab" role="tab" aria-selected={tab==='installed'} aria-controls="installed-panel" disabled={busy||createBusy} onClick={()=>setTab('installed')}>已安装 <small>{data.plugins.length}</small></button>
        <button id="discover-tab" role="tab" aria-selected={tab==='discover'} aria-controls="discover-panel" disabled={busy||createBusy} onClick={()=>{setStoreID('');setTab('discover');}}>发现</button>
        <button id="downloads-tab" role="tab" aria-selected={tab==='downloads'} aria-controls="downloads-panel" disabled={busy||createBusy} onClick={()=>setTab('downloads')}>下载安装 {tasks.some(task=>['downloading','installing'].includes(task.state))&&<small>{tasks.filter(task=>['downloading','installing'].includes(task.state)).length}</small>}</button>
      </div>
      <div className="center-utilities">
        <button disabled={busy||projectBusy} aria-expanded={showInstall} onClick={()=>setShowInstall(value=>!value)}>本地安装</button>
        <button disabled={busy||projectBusy} aria-expanded={showMore} onClick={()=>setShowMore(value=>!value)}>更多</button>
      </div>
    </div>
    {showInstall&&<form className="install" onSubmit={event=>{event.preventDefault();installPackage();}}>
      <label htmlFor="directory">安装 / 更新插件</label>
      <div className="install-row"><input id="directory" spellCheck={false} autoCorrect="off" autoCapitalize="off" autoComplete="off" value={directory} disabled={busy||projectBusy} onChange={event=>changeDirectory(event.target.value)} placeholder="插件目录或 ZIP 的绝对路径" required/><button type="button" disabled={busy||projectBusy} aria-label="选择插件包目录" onClick={()=>void chooseInstallSource()}>选择目录</button><button type="button" disabled={busy||projectBusy} aria-label="选择 ZIP 插件包" onClick={()=>void chooseInstallSource(true)}>选择 ZIP</button><button className="primary" disabled={busy||projectBusy||!directory.trim()}>{busy?'处理中…':'安装 / 更新'}</button></div>
    </form>}
    {tab==='installed'?<div id="installed-panel" role="tabpanel" aria-labelledby="installed-tab">
      <div className="store-toolbar"><input aria-label="搜索已安装插件" placeholder="搜索已安装插件…" value={query} onChange={event=>setQuery(event.target.value)}/><button disabled={busy||projectBusy} onClick={()=>void perform(async()=>{})}>刷新</button></div>
      <div className="store-layout">
        <div className="store-list" aria-label="已安装插件">
          {plugins.map(plugin=><button key={plugin.id} className={`store-item${selected?.id===plugin.id?' selected':''}`} aria-pressed={selected?.id===plugin.id} disabled={busy||projectBusy} onClick={()=>setSelectedID(plugin.id)}><PluginIcon name={plugin.name} icon={plugin.icon}/><span className="store-item-label"><strong>{plugin.name}</strong><small>{plugin.version}{plugin.disabled?' · 已禁用':''}</small></span></button>)}
          {!plugins.length&&<p className="empty">{busy?'正在读取插件…':query?'没有匹配的插件':'还没有安装插件'}</p>}
        </div>
        {selected&&<PluginDetails plugin={selected}>
          <p className="store-note">{selected.version} · {states[selected.state]??selected.state}{selected.development?' · 开发链接':''}{selected.changed?' · 文件有变更':''}</p>
          {!!selected.dependencies?.length&&<div className="store-note">{selected.dependencies.map(dep=><p key={dep.id}>{dep.pluginId} ≥ {dep.minVersion} · {dep.optional?'可选':'必需'} · {dep.state==='available'?'可用':dep.message}<button disabled={busy||projectBusy} onClick={()=>{if(data.plugins.some(p=>p.id===dep.pluginId)){setSelectedID(dep.pluginId);}else{setStoreID(dep.pluginId);setTab('discover');}}}>查看</button></p>)}</div>}
          {!!selected.requiredBy?.length&&<p className="store-note">被依赖：{selected.requiredBy.join('、')}</p>}
          {renderActions(selected.id)}
          {!selected.protected&&!selected.development&&<button className="center-browse" disabled={busy||projectBusy} onClick={()=>{setStoreID(selected.id);setTab('discover');}}>查看商店版本</button>}
        </PluginDetails>}
      </div>
    </div>:tab==='discover'?<div id="discover-panel" role="tabpanel" aria-labelledby="discover-tab"><PluginStore tasks={tasks} bridge={storeBridge} initialID={storeID} plugins={data.plugins} disabled={busy||createBusy} renderActions={renderActions} onBusyChange={setStoreBusy}/></div>:<section id="downloads-panel" role="tabpanel" aria-labelledby="downloads-tab" className="install-tasks">
      {!tasks.length&&<p className="empty">暂无下载安装任务</p>}
      {tasks.slice().reverse().map(task=><div className="install-task" key={task.id}>
        <span>{data.plugins.find(plugin=>plugin.id===task.pluginId)?.name??task.pluginId} · {task.version}</span>
        <span role="status">{{downloading:task.totalBytes>0?(task.downloadedBytes>=task.totalBytes?'校验中…':'下载中…'):'准备下载…',installing:'安装中…',succeeded:'已完成',failed:'失败',cancelled:'已取消'}[task.state]}</span>
        {(task.totalBytes>0||['downloading','installing'].includes(task.state))&&<div className="install-task-progress">
          <progress aria-label={`${task.pluginId} 下载进度`} max={task.totalBytes||1} value={task.totalBytes>0?Math.min(task.downloadedBytes,task.totalBytes):undefined}/>
          {task.totalBytes>0&&<small>{downloadSize(task.downloadedBytes)} / {downloadSize(task.totalBytes)} · {Math.floor(Math.min(task.downloadedBytes/task.totalBytes,1)*100)}%</small>}
        </div>}
        {task.error&&<span className="error">{task.error}</span>}
        {task.state==='downloading'&&<button onClick={()=>void storeBridge.cancelInstall(task.id).catch(reason=>setError(String(reason)))}>取消</button>}
        {['failed','cancelled'].includes(task.state)&&<button disabled={tasks.some(other=>other.pluginId===task.pluginId&&['downloading','installing'].includes(other.state))} onClick={()=>void storeBridge.load(true).then(catalog=>storeBridge.install(task.pluginId,task.version,catalog.revision)).catch(reason=>setError(String(reason)))}>重试</button>}
      </div>)}
    </section>}

    {data.diagnostics.map((diagnostic,index)=><p className="error" key={index}>{diagnostic}</p>)}
    {error&&<p className="error" role="alert">{error}</p>}
    {notice&&<p className="settings-notice" role="status">{notice}</p>}
    {showMore&&<div className="center-more"><button disabled={busy||projectBusy} onClick={()=>setShowCreate(value=>!value)} aria-expanded={showCreate}>创建插件</button><PluginData bridge={bridge} disabled={busy||projectBusy} snapshot={data} /><div className="management-recovery"><button type="button" onClick={onRecover}>恢复入口</button></div></div>}
    {showCreate&&<CreatePlugin bridge={bridge} disabled={busy||storeBusy} onBusyChange={setCreateBusy} onClose={()=>setShowCreate(false)}/> }
  </div>;
}
