// Deterministic browser fixture: real App and Wails event delivery, mocked IPC.
// Providers advance only when the test asks, so slow-machine timing is irrelevant.
import React from 'react';
import { createRoot } from 'react-dom/client';
import { objectNames, setTransport } from '@wailsio/runtime';
import App from '../src/App';
import '../src/style.css';

declare global {
  interface Window {
    _wails: { dispatchWailsEvent(event: { name: string; data: unknown }): void };
  }
}

const commands = ['Alpha', 'Beta', 'Gamma', 'Late'].map(title => ({id:title.toLowerCase(),title}));
const plugin = {id:'example.fixture',name:'Fixture',version:'0.1.0',kind:'process',state:'running',commands};
const item = (title: string) => ({key:`example.fixture/demo/${title}`,pluginId:plugin.id,providerId:'demo',commandId:title.toLowerCase(),input:'fixture',title,subtitle:'Fixture',kind:'process',score:.7});
let sequence = '';
let items = ['Alpha','Beta','Gamma'].map(item);
let done = false;
function publish() {
  window._wails.dispatchWailsEvent({name:'search:update',data:{queryId:sequence,items:[...items],pending:done?0:1,failures:[],done}});
}
const fixture = {
  advance() { items = [item('Late'),...items.filter(item=>item.title!=='Late')]; publish(); },
  complete() { done=true; publish(); },
  refresh() { publish(); },
};
Object.assign(window,{navigationFixture:fixture});
setTransport({async call(object,_method,_window,args){
  if(object!==objectNames.Call)return;
  switch(args.methodID){
    case 2914360659: case 2382139382:return {plugins:[plugin],diagnostics:[]};
    case 1864886877:return '';
    case 1083380061:
      sequence=String(args.args[0]);items=['Alpha','Beta','Gamma'].map(item);done=false;
      queueMicrotask(publish);return;
    case 2263823638:return {text:args.args[2]};
    default:return;
  }
}});
createRoot(document.getElementById('root')!).render(<React.StrictMode><App /></React.StrictMode>);
