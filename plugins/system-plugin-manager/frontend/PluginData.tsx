import {useEffect,useRef,useState} from 'react';

export type PluginDataInfo={id:string;name:string;installed:boolean;bytes:number;corrupt:boolean;token:string};
export type DataBridge={listData():Promise<PluginDataInfo[]>;clearData(id:string,token:string):Promise<void>};
function size(bytes:number){return bytes<1024?`${bytes} B`:bytes<1024*1024?`${(bytes/1024).toFixed(1)} KiB`:`${(bytes/1024/1024).toFixed(1)} MiB`;}

// Keep cleanup in the trusted management plugin. Only metadata enters this view.
export default function PluginData({bridge,disabled,snapshot}:{bridge:DataBridge;disabled:boolean;snapshot:unknown}) {
 const [expanded,setExpanded]=useState(false),[rows,setRows]=useState<PluginDataInfo[]>([]);
 const [loading,setLoading]=useState(false),[clearing,setClearing]=useState(false);
 const [refresh,setRefresh]=useState(0),[confirm,setConfirm]=useState<PluginDataInfo>();
 const [error,setError]=useState(''),[notice,setNotice]=useState('');
 const alive=useRef(false),pending=useRef(false);
 const confirmPanel=useRef<HTMLDivElement>(null),cancelButton=useRef<HTMLButtonElement>(null),opener=useRef<HTMLButtonElement>();
 useEffect(()=>{if(confirm){confirmPanel.current?.scrollIntoView({block:"nearest"});cancelButton.current?.focus({preventScroll:true});}},[confirm]);
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;};},[]);
 useEffect(()=>{
  if(!expanded)return;
  let current=true;setLoading(true);setConfirm(undefined);
  bridge.listData().then(next=>{if(current)setRows(next);})
   .catch(reason=>{if(current)setError(reason instanceof Error?reason.message:'无法读取插件数据。');})
   .finally(()=>{if(current)setLoading(false);});
  return()=>{current=false;};
 },[bridge,expanded,snapshot,refresh]);
 const busy=disabled||loading||clearing;
 async function clear(){
  if(!confirm||busy||pending.current)return;
  pending.current=true;setClearing(true);setError('');setNotice('');
  const selected=confirm;
  try{
   await bridge.clearData(selected.id,selected.token);
   if(alive.current)setNotice(`${selected.name} 的私有数据已清空。`);
  }catch(reason){if(alive.current)setError(reason instanceof Error?reason.message:'清空失败，请重试。');}
  finally{
   pending.current=false;
   if(alive.current){setClearing(false);setConfirm(undefined);setRefresh(value=>value+1);}
  }
 }
 return <section className="plugin-data" aria-label="插件私有数据">
  <div className="data-toolbar"><h2>插件数据</h2><button disabled={busy} aria-expanded={expanded} aria-controls="plugin-data-list" onClick={()=>{setExpanded(!expanded);setConfirm(undefined);setError('');setNotice('');}}>{expanded?'收起':'查看插件数据'}</button></div>
  {expanded&&<div id="plugin-data-list">
   <div className="data-toolbar"><small>{rows.length} 项 · {size(rows.reduce((total,row)=>total+row.bytes,0))}</small><button disabled={busy} onClick={()=>{setError('');setNotice('');setRefresh(value=>value+1);}}>刷新数据</button></div>
   {rows.map(row=><div className="data-row" key={row.id}>
    <div><strong>{row.name}</strong><small>{row.id} · {size(row.bytes)}{!row.installed?' · 未安装':''}{row.corrupt?' · 文件异常':''}</small></div>
    <button className="danger" disabled={busy||!row.token} aria-label={`清空 ${row.name} 的数据`} onClick={event=>{opener.current=event.currentTarget;setConfirm(row);setError('');setNotice('');}}>清空数据</button>
   </div>)}
   {!loading&&!rows.length&&<p className="empty">暂无插件私有数据</p>}
   {confirm&&<div ref={confirmPanel} className="data-confirm" role="group" aria-label={`清空 ${confirm.name} 的数据确认`}>
    <p>清空 {confirm.name} 的私有数据？</p><small>会关闭插件页面。设置和使用历史保留，此操作不可撤销。</small>
    <div><button ref={cancelButton} disabled={busy} onClick={()=>{setConfirm(undefined);opener.current?.focus();}}>取消</button><button className="danger" disabled={busy} onClick={()=>void clear()}>确认清空</button></div>
   </div>}
   {error&&<p className="error" role="alert">{error}</p>}
   {notice&&<p className="settings-notice" role="status">{notice}</p>}
  </div>}
 </section>;
}
