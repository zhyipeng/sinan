import { useEffect, useRef, useState } from 'react';
import { RepairSystemPlugin, PreviewSystemRepair, SetPluginEnabled } from '../bindings/sinan/apps/desktop/hostservice';
import type { PackagePreview } from '../../../../plugins/system-plugin-manager/frontend/PluginManager';

// Minimal recovery is always bundled with the Shell and does not require an
// installed management plugin. Daily lifecycle controls belong to that plugin.
export default function Recovery({available,disabled,diagnostics,shortcutError,onRefresh,onOpen}: {available:boolean;disabled:boolean;diagnostics:string[];shortcutError:string;onRefresh():Promise<void>;onOpen():void}) {
  const [directory,setDirectory]=useState('');
  const [preview,setPreview]=useState<PackagePreview>();
  const [busy,setBusy]=useState(false);
  const [error,setError]=useState('');
  const [notice,setNotice]=useState('');
  const alive=useRef(false);
  useEffect(()=>{alive.current=true;return()=>{alive.current=false;};},[]);
  async function perform(action:()=>Promise<void>){
    if(busy)return;
    setBusy(true);setError('');setNotice('');
    try{await action();await onRefresh();}
    catch(error){if(alive.current)setError(error instanceof Error?error.message:'恢复失败，请重试。');}
    finally{if(alive.current)setBusy(false);}
  }
  return <div className="recovery tool-body">
    <p className="settings-note">{disabled?'插件管理已禁用，可重新启用。':available?'插件管理已安装，可重新检查状态或恢复本地包。':'插件管理不可用，请从本地目录恢复。'}</p>
    <div className="recovery-actions"><button disabled={busy} onClick={()=>void perform(async()=>{})}>重新检查</button>{available&&<button disabled={busy} onClick={onOpen}>打开插件管理</button>}{disabled&&<button disabled={busy} onClick={()=>void perform(async()=>{await SetPluginEnabled('system.plugin-manager',true);})}>启用插件管理</button>}</div>
    <form className="install" onSubmit={event=>{
      event.preventDefault();
      void perform(async()=>{
        if(!preview){
          const next=await PreviewSystemRepair(directory);
          if(alive.current)setPreview(next);
        }else if(preview.changed){
          try{
            await RepairSystemPlugin(directory,preview.fingerprint,preview.installedFingerprint);
            if(alive.current){setDirectory('');setNotice('系统插件已修复。');}
          }finally{if(alive.current)setPreview(undefined);}
        }
      });
    }}>
      <label htmlFor="recovery-directory">修复系统插件</label>
      <div className="install-row"><input id="recovery-directory" value={directory} disabled={busy} spellCheck={false} autoCorrect="off" autoCapitalize="off" autoComplete="off" onChange={event=>{setDirectory(event.target.value);setPreview(undefined);setError('');setNotice('');}} placeholder="插件目录的绝对路径" required/><button className="primary" disabled={busy||!directory.trim()||preview&&!preview.changed}>{!preview?'检查':preview.changed?'恢复':'无需更新'}</button></div>
      {preview&&<div className="package-preview" role="status"><strong>{preview.name}</strong><span>{preview.version}</span></div>}
    </form>
    {diagnostics.map((message,index)=><p className="error" key={index}>{message}</p>)}
    {shortcutError&&<p className="error">{shortcutError}</p>}
    {error&&<p className="error" role="alert">{error}</p>}
    {notice&&<p className="settings-notice" role="status">{notice}</p>}
  </div>;
}
