import { useEffect, useRef, useState } from 'react';
import { RecordIsolationProbe, ResizeLauncher } from '../bindings/sinan/apps/desktop/hostservice';

type Report = { parentBlocked: boolean; bindingsAbsent: boolean; storageBlocked: boolean; runtimeBlocked: boolean; resourceBlocked: boolean; networkBlocked: boolean };
const reportKeys = ['parentBlocked', 'bindingsAbsent', 'storageBlocked', 'runtimeBlocked', 'resourceBlocked', 'networkBlocked'] as const;

// This is a fixed diagnostic fixture, not a general-purpose plugin HTML loader.
const guest = `<!doctype html><html><head><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src 'none'; img-src 'none'; frame-src 'none'; form-action 'none'; base-uri 'none'"><style>body{font:14px -apple-system,sans-serif;padding:16px;color:#253047}button{padding:8px 16px}</style></head><body><p>隔离页面</p><input aria-label="隔离页输入" placeholder="可测试输入法"><button id="close">关闭</button><button id="navigate">导航探测</button><script>
let port;
addEventListener('message',async event=>{
 if(port || event.source!==parent || event.data?.kind!=='init' || event.ports.length!==1)return;
 port=event.ports[0];
 let parentBlocked=false,storageBlocked=false;
 try{parent.document.title}catch{parentBlocked=true}
 try{localStorage.getItem('probe')}catch{storageBlocked=true}
 const blocked=async (url,post=false)=>{try{await fetch(url,post?{method:'POST',headers:{'Content-Type':'application/json'},body:'{}'}:{});return false}catch{return true}};
 const runtimeBlocked=await blocked(event.data.origin+'/wails/runtime',true);
 const resourceBlocked=await blocked(event.data.origin+'/plugins/example.other/private.json');
 const networkBlocked=event.data.networkControl===true && await blocked('data:application/json,{}');
 port.postMessage({kind:'execute',pluginId:'example.hello'});
 port.postMessage({kind:'ready',report:{parentBlocked,storageBlocked,runtimeBlocked,resourceBlocked,networkBlocked,bindingsAbsent:typeof window._wails==='undefined'&&typeof window.__WAILS__==='undefined'}});
 document.getElementById('close').onclick=()=>port.postMessage({kind:'close'});
 document.getElementById('navigate').onclick=()=>location.replace('about:blank');
});
</script></body></html>`;

export default function IsolationLab() {
  const frame = useRef<HTMLIFrameElement>(null);
  const channel = useRef<MessageChannel>();
  const loads = useRef(0);
  const rejected = useRef(0);
  const [report, setReport] = useState<Report>();
  const [closed, setClosed] = useState(false);
  const [error, setError] = useState('');
  useEffect(() => { void ResizeLauncher(400).catch(() => {}); return () => {channel.current?.port1.close();channel.current?.port2.close();}; }, []);
  async function loaded() {
    // A second load is a navigation attempt. Tear down the guest rather than
    // granting a new document the original message channel.
    if (++loads.current !== 1) { setError('已阻止客体导航'); setClosed(true); channel.current?.port1.close(); return; }
    const connection = new MessageChannel();
    channel.current = connection;
    connection.port1.onmessage = event => {
      const message = event.data;
      if (message?.kind === 'close' && Object.keys(message).length === 1) {connection.port1.close();setClosed(true);return;}
      if (message?.kind !== 'ready' || !message.report || reportKeys.some(key => typeof message.report[key] !== 'boolean') || Object.keys(message.report).length !== reportKeys.length) {rejected.current++;return;}
      const result = Object.fromEntries(reportKeys.map(key => [key, message.report[key]])) as Report;
      setReport(result);
      void RecordIsolationProbe(result, rejected.current > 0).catch(() => {});
    };
    const networkControl = await fetch('data:application/json,{}').then(response => response.ok).catch(() => false);
    if (channel.current !== connection) return;
    frame.current?.contentWindow?.postMessage({kind:'init',origin:window.location.origin,networkControl}, '*', [connection.port2]);
  }
  return <main style={{padding:20}}>
    <h1 style={{fontSize:18}}>UI 隔离验证</h1>
    <p>仅用于开发验证，不加载第三方插件。</p>
    {!closed && <iframe ref={frame} title="隔离测试页" sandbox="allow-scripts" referrerPolicy="no-referrer" srcDoc={guest} onLoad={loaded} style={{width:'100%',height:140,border:'1px solid #e5e8f0'}} />}
    <p role="status">{error || (closed ? '已关闭隔离页面' : report ? '验证完成' : '验证中…')}</p>
    {report && <ul>{reportKeys.map(key=><li key={key}>{key}：{report[key] ? '通过' : '失败'}</li>)}<li>非法动作拒绝：{rejected.current > 0 ? '通过' : '失败'}</li></ul>}
  </main>;
}
