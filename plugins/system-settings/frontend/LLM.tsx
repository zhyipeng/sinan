import {useEffect,useRef,useState} from 'react';
export type LLMProfile={id:string;name:string;baseURL:string;model:string;hasKey:boolean};
export type LLMState={revision:number;profiles:LLMProfile[];defaultProfile:string;overrides:Record<string,string>};
export type LLMEdit={revision:number;id:string;name:string;baseURL:string;model:string;apiKey:string;clearKey:boolean};
export type LLMBridge={load():Promise<LLMState>;save(edit:LLMEdit):Promise<LLMState>;remove(id:string,revision:number):Promise<LLMState>;preferences(value:{revision:number;defaultProfile:string;overrides:Record<string,string>}):Promise<LLMState>};
const empty={id:'',name:'',baseURL:'',model:'',apiKey:'',clearKey:false};
export default function LLM({bridge,plugins}:{bridge:LLMBridge;plugins:{id:string;name:string}[]}){
 const [state,setState]=useState<LLMState>(),[draft,setDraft]=useState(empty),[pending,setPending]=useState(false),[error,setError]=useState(''),[notice,setNotice]=useState('');
 const alive=useRef(true),busy=useRef(false);
 useEffect(()=>{alive.current=true;void bridge.load().then(value=>{if(alive.current)setState(old=>!old||value.revision>=old.revision?value:old);}).catch(()=>{if(alive.current)setError('无法读取全局 LLM 配置。');});return()=>{alive.current=false;};},[bridge]);
 async function perform(action:()=>Promise<LLMState>,clear=false){if(busy.current)return;busy.current=true;setPending(true);setError('');setNotice('');try{const next=await action();if(alive.current){setState(old=>!old||next.revision>=old.revision?next:old);if(clear)setDraft(empty);setNotice('已保存。');}}catch(e){if(alive.current)setError(e instanceof Error?e.message:'保存失败。');}finally{busy.current=false;if(alive.current)setPending(false);}}
 function edit(id:string){const profile=state?.profiles.find(p=>p.id===id);setDraft(profile?{id:profile.id,name:profile.name,baseURL:profile.baseURL,model:profile.model,apiKey:'',clearKey:false}:empty);setError('');setNotice('');}
 function prefs(defaultProfile:string,overrides:Record<string,string>){if(!state)return;void perform(()=>bridge.preferences({revision:state.revision,defaultProfile,overrides}));}
 const selected=state?.profiles.find(p=>p.id===draft.id);
 return <section className="settings-section" aria-label="全局 LLM 配置"><h2>LLM</h2>
  <div className="settings-field"><label htmlFor="llm-default">全局默认配置</label><select id="llm-default" disabled={!state||pending} value={state?.defaultProfile??''} onChange={e=>state&&prefs(e.target.value,state.overrides)}>{!state?.profiles.length&&<option value="">尚未配置</option>}{state?.profiles.map(p=><option key={p.id} value={p.id}>{p.name}</option>)}</select></div>
  <div className="settings-field"><label htmlFor="llm-edit">配置管理</label><select id="llm-edit" disabled={!state||pending} value={draft.id} onChange={e=>edit(e.target.value)}><option value="">新建配置</option>{state?.profiles.map(p=><option key={p.id} value={p.id}>{p.name}</option>)}</select></div>
  <form onSubmit={e=>{e.preventDefault();if(!state||pending)return;const value={...draft,revision:state.revision};setDraft(d=>({...d,apiKey:''}));void perform(()=>bridge.save(value),true);}}>
   {([['name','配置名称','例如：日常使用'],['baseURL','API 地址','https://example.com/v1'],['model','模型','填写模型名称']] as const).map(([field,label,placeholder])=><div className="settings-field" key={field}><label htmlFor={`llm-${field}`}>{label}</label><input id={`llm-${field}`} type="text" disabled={!state||pending} value={draft[field]} required maxLength={field==='name'?64:field==='model'?128:2048} autoComplete="off" placeholder={placeholder} onChange={e=>setDraft(d=>({...d,[field]:e.target.value}))}/></div>)}
   <div className="settings-field"><label htmlFor="llm-key">API Key</label><input id="llm-key" className="llm-key" type="password" disabled={!state||pending} value={draft.apiKey} maxLength={2048} autoComplete="new-password" placeholder={selected?.hasKey?'已保存，留空保留':'可留空（免认证接口）'} onChange={e=>setDraft(d=>({...d,apiKey:e.target.value}))}/></div>
   {selected?.hasKey&&<label><input type="checkbox" disabled={pending} checked={draft.clearKey} onChange={e=>setDraft(d=>({...d,clearKey:e.target.checked}))}/>清除已保存的 Key</label>}
   <div className="settings-save"><button type="button" disabled={!state||pending} onClick={()=>void perform(()=>bridge.load(),true)}>重新载入</button><div>{draft.id&&<button type="button" disabled={pending} onClick={()=>state&&void perform(()=>bridge.remove(draft.id,state.revision),true)}>删除</button>} <button className="primary" disabled={!state||pending}>保存配置</button></div></div>
  </form>
  {!!state?.profiles.length&&<><h2>插件默认配置</h2>{plugins.map(plugin=><div className="settings-field" key={plugin.id}><label htmlFor={`llm-plugin-${plugin.id}`}>{plugin.name}</label><select id={`llm-plugin-${plugin.id}`} disabled={pending} value={state.overrides[plugin.id]??''} onChange={e=>prefs(state.defaultProfile,{...state.overrides,[plugin.id]:e.target.value})}><option value="">跟随全局默认</option>{state.profiles.map(p=><option key={p.id} value={p.id}>{p.name}</option>)}</select></div>)}</>}
  {notice&&<p className="settings-notice" role="status">{notice}</p>}{error&&<p className="error" role="alert">{error}</p>}
 </section>;
}
