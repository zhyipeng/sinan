import { useEffect, useRef, useState } from 'react';

export type StartupState = { silent:boolean; revision:number; error?:string; autoStart:boolean; autoStartError:string };
export type StartupBridge = {
  load():Promise<StartupState>;
  silent(enabled:boolean, revision:number):Promise<StartupState>;
  autoStart(enabled:boolean):Promise<StartupState>;
  subscribe(changed:(state:StartupState)=>void):()=>void;
};

export default function Startup({bridge}:{bridge:StartupBridge}) {
  const [state,setState]=useState<StartupState>();
  const [pending,setPending]=useState(false);
  const [error,setError]=useState('');
  const saving=useRef(false), alive=useRef(false);
  useEffect(()=>{
    alive.current=true;
    let changed=false;
    const off=bridge.subscribe(next=>{changed=true;setState(next);});
    void bridge.load().then(next=>{if(alive.current&&!changed)setState(next);})
      .catch(()=>{if(alive.current)setError('无法读取启动设置。');});
    return ()=>{alive.current=false;off();};
  },[bridge]);
  async function save(kind:'silent'|'autoStart',enabled:boolean) {
    if(!state||saving.current)return;
    saving.current=true;setPending(true);setError('');
    try {
      const next=await (kind==='silent'?bridge.silent(enabled,state.revision):bridge.autoStart(enabled));
      if(alive.current)setState(next);
    } catch(reason) {
      if(alive.current) {
        setError(reason instanceof Error?reason.message:'启动设置保存失败。');
        try {const next=await bridge.load();if(alive.current)setState(next);}catch{/* 保留保存失败提示。 */}
      }
    } finally {saving.current=false;if(alive.current)setPending(false);}
  }
  return <section className="settings-section" aria-label="启动">
    <h2>启动</h2>
    <label className="search-open-toggle"><span>静默启动</span>
      <input type="checkbox" role="switch" checked={state?.silent??false} disabled={!state||pending||!!state.error}
        onChange={event=>void save('silent',event.target.checked)}/></label>
    <p className="settings-note">下次启动时隐藏主输入框</p>
    <label className="search-open-toggle"><span>开机自启</span>
      <input type="checkbox" role="switch" checked={state?.autoStart??false} disabled={!state||pending||!!state.autoStartError}
        onChange={event=>void save('autoStart',event.target.checked)}/></label>
    {(error||state?.error||state?.autoStartError)&&<p className="error" role="alert">{error||state?.error||state?.autoStartError}</p>}
  </section>;
}
