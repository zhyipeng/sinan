import PluginIcon from '../../../packages/ui-components/PluginIcon';
import { useEffect, useRef, useState } from 'react';
import CreatePlugin, {type ProjectBridge} from './CreatePlugin';
import PluginData,{type DataBridge} from './PluginData';

export type ManagedPlugin = {icon?: string;development?: boolean; developmentFingerprint?: string; needsTrust?: boolean; protected?: boolean; id: string; name: string; version: string; kind: string; state: string; changed?: boolean; disabled?: boolean; uiViews?: {id:string;title?:string}[] | null};
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
const states: Record<string,string> = {installed:'已安装',activating:'启动中',activated:'已激活',running:'运行中',deactivated:'已停止',failed:'运行失败',disabled:'已禁用'};

export default function PluginManager({bridge, onChanged, onRecover,onOpenStore}: {bridge: ManagementBridge; onChanged(): Promise<void>; onRecover(): void;onOpenStore?():void}) {
  const [data,setData]=useState<ManagedSnapshot>({plugins:[],diagnostics:[]});
  const [busy,setBusy]=useState(true);
  const [showCreate,setShowCreate]=useState(false);
  const [projectBusy,setProjectBusy]=useState(false);
  const [directory,setDirectory]=useState('');
  const [preview,setPreview]=useState<PackagePreview>();
  const [trust,setTrust]=useState(false);
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

  async function perform(action:()=>Promise<unknown>, message='') {
    if(pending.current||projectBusy)return;
    pending.current=true;setBusy(true);setError('');setNotice('');
    try {
      await action();
      const next=await bridge.refresh();
      // Keep the Shell's command registry current even if this view was closed.
      await onChanged();
      if(alive.current){setData(next);setNotice(message);}
    } catch(error) {if(alive.current)setError(error instanceof Error?error.message:'操作失败，请重试。');}
    finally {pending.current=false;if(alive.current)setBusy(false);}
  }
  function inspect() {
    setPreview(undefined);setTrust(false);
    void perform(async()=>{
      const next=await bridge.inspect(directory);
      if(alive.current)setPreview(next);
    });
  }
  function changeDirectory(value: string) {
    setDirectory(value);setPreview(undefined);setTrust(false);setError('');setNotice('');
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
  return <div className="management management-body">
    <div className="management-toolbar"><span>{data.plugins.length} 个插件</span><button disabled={busy||projectBusy} onClick={()=>void perform(async()=>{})}>刷新</button><button disabled={busy||projectBusy} onClick={()=>setShowCreate(value=>!value)} aria-expanded={showCreate}>创建插件</button>{onOpenStore&&<button disabled={busy||projectBusy} onClick={onOpenStore}>插件商店</button>}</div>
    <div className="plugins">
      {data.plugins.map(plugin=><div className="plugin" key={plugin.id}>
        <div className="plugin-identity"><PluginIcon name={plugin.name} icon={plugin.icon}/><div><strong>{plugin.name}</strong><small>{plugin.version}{plugin.development?' · 开发链接':''} · {states[plugin.state]??plugin.state}{plugin.changed?' · 文件有变更':''}</small></div></div>
        <div className="plugin-actions">
          {plugin.needsTrust&&<><label className="trust"><input type="checkbox" disabled={busy||projectBusy} checked={!!developmentTrust[plugin.id]} onChange={event=>setDevelopmentTrust(previous=>({...previous,[plugin.id]:event.target.checked}))}/>允许以当前用户权限运行</label><button disabled={busy||projectBusy||!developmentTrust[plugin.id]||plugin.changed} onClick={()=>void perform(async()=>{await bridge.trustDevelopment(plugin.id,plugin.developmentFingerprint!);setDevelopmentTrust(previous=>({...previous,[plugin.id]:false}));})}>信任开发插件</button></>}

          <button disabled={busy||projectBusy||!['running','activated','failed'].includes(plugin.state)} aria-label={`停止 ${plugin.name}`} onClick={()=>void perform(()=>bridge.stop(plugin.id))}>停止</button>
          {!plugin.protected&&<button className="danger" disabled={busy||projectBusy} aria-label={`卸载 ${plugin.name}`} onClick={()=>void perform(()=>bridge.uninstall(plugin.id))}>卸载</button>}
          <button disabled={busy||projectBusy} aria-label={`重载 ${plugin.name}`} onClick={()=>void perform(()=>bridge.reload(plugin.id))}>重载</button>
          {!plugin.protected&&<button disabled={busy||projectBusy} aria-label={`${plugin.disabled?'启用':'禁用'} ${plugin.name}`} onClick={()=>void perform(()=>bridge.setEnabled(plugin.id,!!plugin.disabled))}>{plugin.disabled?'启用':'禁用'}</button>}
          {data.uiEnabled&&plugin.uiViews?.map(view=><button key={view.id} disabled={busy||projectBusy||plugin.disabled||plugin.changed} aria-label={`打开 ${plugin.name} ${view.title||view.id}`} onClick={()=>void perform(()=>bridge.openView!(plugin.id,view.id))}>打开 {view.title||view.id}</button>)}
        </div>
      </div>)}
      {!busy&&!data.plugins.length&&<p className="empty">还没有插件，从本地目录或 ZIP 安装一个。</p>}
    </div>
    <form className="install" onSubmit={event=>{
      event.preventDefault();
      if(busy)return;
      if(!preview){inspect();return;}
      if(!preview.changed||preview.kind==='process'&&!trust)return;
      void perform(async()=>{
        try {
          await bridge.apply(directory,trust,preview.fingerprint,preview.installedFingerprint);
          if(alive.current)setDirectory('');
        } finally {if(alive.current){setPreview(undefined);setTrust(false);}}
      },preview.installedVersion?'插件已升级。':'插件已安装。');
    }}>
      <label htmlFor="directory">安装 / 升级插件</label>
      <div className="install-row"><input id="directory" spellCheck={false} autoCorrect="off" autoCapitalize="off" autoComplete="off" value={directory} disabled={busy||projectBusy} onChange={event=>changeDirectory(event.target.value)} placeholder="插件目录或 ZIP 的绝对路径" required/><button type="button" disabled={busy||projectBusy} aria-label="选择插件包目录" onClick={()=>void chooseInstallSource()}>选择目录</button><button type="button" disabled={busy||projectBusy} aria-label="选择 ZIP 插件包" onClick={()=>void chooseInstallSource(true)}>选择 ZIP</button><button className="primary" disabled={busy||projectBusy||!directory.trim()||!!preview&&(!preview.changed||preview.kind==='process'&&!trust)}>{!preview?'检查':!preview.changed?'无需升级':preview.installedVersion?'升级':'安装'}</button></div>
      {preview&&<div className="package-preview" role="status"><strong>{preview.name}</strong><span>{preview.installedVersion?`${preview.installedVersion} → ${preview.version}`:preview.version}</span><button type="button" disabled={busy||projectBusy} onClick={inspect}>重新检查</button></div>}
      {preview?.kind==='process'&&preview.changed&&<label className="trust"><input type="checkbox" checked={trust} onChange={event=>setTrust(event.target.checked)}/>信任此原生插件，允许其以当前用户权限运行</label>}
    </form>
    {data.diagnostics.map((diagnostic,index)=><p className="error" key={index}>{diagnostic}</p>)}
    {error&&<p className="error" role="alert">{error}</p>}
    {notice&&<p className="settings-notice" role="status">{notice}</p>}
    <div className="management-recovery"><button type="button" onClick={onRecover}>恢复入口</button></div>
    <PluginData bridge={bridge} disabled={busy||projectBusy} snapshot={data} />
    {showCreate&&<CreatePlugin bridge={bridge} disabled={busy} onBusyChange={setProjectBusy} onClose={()=>setShowCreate(false)}/> }
  </div>;
}
