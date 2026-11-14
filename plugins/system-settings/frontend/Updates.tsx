import {useEffect,useState} from 'react';

export type UpdateState={currentVersion:string;latestVersion:string;phase:string;message:string;available:boolean;supported:boolean;installationResult:string};
export type UpdateBridge={load():Promise<UpdateState>;check():Promise<UpdateState>;install():Promise<void>;subscribe(changed:(state:UpdateState)=>void):()=>void};
function useUpdateState(bridge:UpdateBridge){
 const [state,setState]=useState<UpdateState>();
 useEffect(()=>{let active=true,sequence=0;const off=bridge.subscribe(next=>{sequence++;if(active)setState(next);});const initial=sequence;void bridge.load().then(next=>{if(active&&initial===sequence)setState(next);}).catch(()=>{});return()=>{active=false;off();};},[bridge]);
 return [state,setState] as const;
}
export function UpdateNotice({bridge,onOpen}:{bridge:UpdateBridge;onOpen:()=>void}){
 const [state]=useUpdateState(bridge);
 if(!state?.available)return null;
 return <button className="update-notice" title={`司南 ${state.latestVersion} 可用`} onClick={onOpen}>有更新</button>;
}
export default function Updates({bridge}:{bridge:UpdateBridge}){
 const [state,setState]=useUpdateState(bridge),[error,setError]=useState(''),[pending,setPending]=useState(false);
 const busy=pending||state?.phase==='checking'||state?.phase==='downloading'||state?.phase==='installing';
 async function check(){setPending(true);setError('');try{setState(await bridge.check());}catch{setError('检查更新失败，请稍后重试。');}finally{setPending(false);}}
 async function install(){setPending(true);setError('');try{await bridge.install();}catch{setError('更新未完成，请检查状态后重试。');}finally{setPending(false);}}
 const message=state?.phase==='checking'?'正在检查更新…':state?.phase==='downloading'?'正在下载并校验…':state?.phase==='installing'?'正在退出并安装…':state?.message||(state?.available?`新版本 ${state.latestVersion} 可用`:'启动后自动检查稳定版本');
 return <section className="settings-section" aria-label="司南更新">
  <h2>司南更新{state?.currentVersion&&<small className="settings-note"> · {state.currentVersion}</small>}</h2>
  <p className="settings-note" role="status">{message}</p>
  {state?.installationResult&&<p className="settings-note">{state.installationResult}</p>}
  <div className="settings-save"><button disabled={!state?.supported||busy} onClick={()=>void check()}>检查更新</button>{state?.available&&<button className="primary" disabled={busy} onClick={()=>void install()}>下载并重启安装</button>}</div>
  {error&&<p className="error" role="alert">{error}</p>}
 </section>;
}
