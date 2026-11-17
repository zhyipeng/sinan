import {useEffect, useRef, useState} from 'react';

export type WindowOpacityState = {percent:number; revision:number; error?:string};
export type WindowOpacityBridge = {
  load():Promise<WindowOpacityState>;
  save(percent:number, revision:number):Promise<WindowOpacityState>;
  subscribe(changed:(state:WindowOpacityState)=>void):()=>void;
};

export default function WindowOpacity({bridge}:{bridge:WindowOpacityBridge}) {
  const [state,setState]=useState<WindowOpacityState>();
  const [draft,setDraft]=useState(100);
  const [error,setError]=useState('');
  const latest=useRef<WindowOpacityState>();
  const requested=useRef<number>();
  const saving=useRef(false),alive=useRef(false);

  function receive(next:WindowOpacityState) {
    if(next.revision<(latest.current?.revision??-1))return;
    latest.current=next;
    if(alive.current) {
      setState(next);
      if(!saving.current)setDraft(next.percent);
    }
  }
  useEffect(()=>{
    alive.current=true;
    const off=bridge.subscribe(receive);
    void bridge.load().then(receive).catch(()=>{if(alive.current)setError('无法读取窗口透明度。');});
    return ()=>{alive.current=false;off();};
  },[bridge]);

  async function change(percent:number) {
    requested.current=percent;
    setDraft(percent);
    if(saving.current)return;
    saving.current=true;
    setError('');
    try {
      // Coalesce rapid dragging, and serialize saves with the returned revision.
      while(requested.current!==undefined) {
        const value=requested.current;
        requested.current=undefined;
        const current=latest.current;
        if(current&&value!==current.percent)receive(await bridge.save(value,current.revision));
      }
    } catch(reason) {
      requested.current=undefined;
      if(alive.current)setError(reason instanceof Error?reason.message:'透明度保存失败。');
      try {receive(await bridge.load());}catch{/* 保留保存错误。 */}
    } finally {
      saving.current=false;
      if(alive.current&&latest.current)setDraft(latest.current.percent);
    }
  }

  return <section className="settings-section" aria-label="窗口透明度">
    <div className="window-opacity-heading"><label htmlFor="main-window-transparency">主窗口透明度</label>
      <output htmlFor="main-window-transparency">{100-draft}%</output></div>
    <input id="main-window-transparency" className="window-opacity-slider" type="range" min="0" max="70" step="1"
      value={100-draft} aria-valuetext={`${100-draft}% 透明`} disabled={!state||!!state.error}
      onChange={event=>void change(100-Number(event.target.value))}/>
    {(error||state?.error)&&<p className="error" role="alert">{error||state?.error}</p>}
  </section>;
}
