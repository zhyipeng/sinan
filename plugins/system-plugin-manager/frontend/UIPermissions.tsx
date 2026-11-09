import {useEffect,useRef,useState} from 'react';

export type UIPermission={id:string;title:string;allowed:boolean};
export type UIPermissionInfo={id:string;name:string;fingerprint:string;reviewed:boolean;stale:boolean;changed:boolean;permissions:UIPermission[]};
export type UIPermissionState={revision:number;plugins:UIPermissionInfo[];error:string};
export type PermissionBridge={uiPermissions():Promise<UIPermissionState>;saveUIPermissions(id:string,fingerprint:string,revision:number,capabilities:string[]):Promise<UIPermissionState>};
export default function UIPermissions({bridge,disabled,snapshot}:{bridge:PermissionBridge;disabled:boolean;snapshot:unknown}){
 const [expanded,setExpanded]=useState(false),[data,setData]=useState<UIPermissionState>();
 const [drafts,setDrafts]=useState<Record<string,string[]>>({}),[refresh,setRefresh]=useState(0);
 const [loading,setLoading]=useState(false),[saving,setSaving]=useState(false);
 const [error,setError]=useState(''),[notice,setNotice]=useState('');
 const alive=useRef(false),pending=useRef(false);
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;};},[]);
 useEffect(()=>{
  if(!expanded)return;
  let current=true;setLoading(true);setDrafts({});
  bridge.uiPermissions().then(next=>{if(current)setData(next);})
   .catch(reason=>{if(current)setError(reason instanceof Error?reason.message:'无法读取 UI 权限。');})
   .finally(()=>{if(current)setLoading(false);});
  return()=>{current=false;};
 },[bridge,expanded,snapshot,refresh]);
 const busy=disabled||loading||saving;
 function selected(plugin:UIPermissionInfo){return drafts[plugin.id]??plugin.permissions.filter(cap=>cap.allowed).map(cap=>cap.id);}
 async function save(plugin:UIPermissionInfo){
  if(!data||busy||pending.current||data.error||plugin.changed)return;
  pending.current=true;setSaving(true);setError('');setNotice('');
  try{
   const next=await bridge.saveUIPermissions(plugin.id,plugin.fingerprint,data.revision,selected(plugin));
   if(alive.current){setData(next);setDrafts({});setNotice('UI 权限已保存，重新打开插件页面后生效。');}
  }catch(reason){if(alive.current)setError(reason instanceof Error?reason.message:'权限保存失败。');}
  finally{pending.current=false;if(alive.current)setSaving(false);}
 }
 return <section className="plugin-permissions" aria-label="插件 UI 权限">
  <div className="data-toolbar"><h2>UI 权限</h2><button disabled={busy} aria-expanded={expanded} aria-controls="ui-permissions-list" onClick={()=>{setExpanded(!expanded);setError('');setNotice('');}}>{expanded?'收起权限':'管理 UI 权限'}</button></div>
  {expanded&&<div id="ui-permissions-list">
   <div className="data-toolbar"><small>仅管理页面能力，原生插件仍按安装信任运行。</small><button disabled={busy} onClick={()=>{setError('');setNotice('');setRefresh(value=>value+1);}}>刷新权限</button></div>
   {data?.plugins.map(plugin=><div className="permission-plugin" key={plugin.id}>
    <div className="permission-title"><strong>{plugin.name}</strong><small>{plugin.changed?'文件有变更，请先重载':drafts[plugin.id]?'待保存':plugin.stale?'包已更新，请重新确认':plugin.reviewed?'已确认':'尚未授权'}</small></div>
    {plugin.permissions.map(cap=><label className="permission-option" key={cap.id}><input type="checkbox" checked={selected(plugin).includes(cap.id)} disabled={busy||!!data.error||plugin.changed} aria-label={`${plugin.name} ${cap.title}`} onChange={event=>{const enabled=event.target.checked;setDrafts(current=>{const previous=current[plugin.id]??plugin.permissions.filter(item=>item.allowed).map(item=>item.id);return {...current,[plugin.id]:enabled?[...new Set([...previous,cap.id])]:previous.filter(id=>id!==cap.id)};});setNotice('');}}/>{cap.title}</label>)}
    <div className="permission-actions"><button disabled={busy||!!data.error||plugin.changed} aria-label={`取消勾选 ${plugin.name} 的全部 UI 权限`} onClick={()=>{setDrafts(current=>({...current,[plugin.id]:[]}));setNotice('');}}>全部取消勾选</button><button className="primary" disabled={busy||!!data.error||plugin.changed} aria-label={`保存 ${plugin.name} 的 UI 权限`} onClick={()=>void save(plugin)}>保存权限</button></div>
   </div>)}
   {data&&!loading&&!data.plugins.length&&<p className="empty">暂无需要授权的 UI 能力</p>}
   {(error||data?.error)&&<p className="error" role="alert">{error||data?.error}</p>}
   {notice&&<p className="settings-notice" role="status">{notice}</p>}
  </div>}
 </section>;
}
