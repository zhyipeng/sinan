import {useEffect,useRef,useState} from 'react';

export type RetentionState={clearInputSeconds:number;pluginUISeconds:number;revision:number;error?:string};
export type RetentionBridge={
  load():Promise<RetentionState>;
  save(clearInputSeconds:number,pluginUISeconds:number,revision:number):Promise<RetentionState>;
  subscribe(changed:(state:RetentionState)=>void):()=>void;
};

export default function Retention({bridge}:{bridge:RetentionBridge}){
  const [state,setState]=useState<RetentionState>();
  const [clear,setClear]=useState('0'),[plugin,setPlugin]=useState('0');
  const [pending,setPending]=useState(false),[error,setError]=useState('');
  const alive=useRef(false),saving=useRef(false),revision=useRef(-1);
  function receive(next:RetentionState){
    if(next.revision<revision.current)return;
    revision.current=next.revision;
    if(alive.current){setState(next);setClear(String(next.clearInputSeconds));setPlugin(String(next.pluginUISeconds));}
  }
  useEffect(()=>{
    alive.current=true;
    const off=bridge.subscribe(receive);
    void bridge.load().then(receive).catch(()=>{if(alive.current)setError('无法读取主界面保留设置。');});
    return()=>{alive.current=false;off();};
  },[bridge]);
  async function save(){
    if(!state||saving.current)return;
    saving.current=true;setPending(true);setError('');
    try{receive(await bridge.save(Number(clear),Number(plugin),state.revision));}
    catch(reason){
      if(alive.current)setError(reason instanceof Error?reason.message:'保留设置保存失败。');
      try{receive(await bridge.load());}catch{/* 保留保存错误。 */}
    }finally{saving.current=false;if(alive.current)setPending(false);}
  }
  return <form className="settings-section" onSubmit={event=>{event.preventDefault();void save();}} aria-label="主界面保留">
    <h2>主界面保留</h2>
    <div className="settings-number-row"><label htmlFor="clear-input-delay">清除输入框延迟</label><div><input id="clear-input-delay" type="number" min="0" max="86400" step="1" required value={clear} disabled={!state||pending||!!state.error} onChange={event=>setClear(event.target.value)}/><span>秒</span></div></div>
    <div className="settings-number-row"><label htmlFor="plugin-ui-retention">插件 UI 保留时长</label><div><input id="plugin-ui-retention" type="number" min="0" max="86400" step="1" required value={plugin} disabled={!state||pending||!!state.error} onChange={event=>setPlugin(event.target.value)}/><span>秒</span></div></div>
    <div className="settings-save"><span className="settings-note">从主界面隐藏时计时，0 表示不保留</span><button className="primary" disabled={!state||pending||!!state.error||(clear===String(state.clearInputSeconds)&&plugin===String(state.pluginUISeconds))}>{pending?'保存中…':'保存'}</button></div>
    {(error||state?.error)&&<p className="error" role="alert">{error||state?.error}</p>}
  </form>;
}
