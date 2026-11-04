// Deterministic browser fixture: real App and Wails event delivery, mocked IPC.
// Providers advance only when the test asks, so slow-machine timing is irrelevant.
import React from 'react';
import { createRoot } from 'react-dom/client';
import { objectNames, setTransport } from '@wailsio/runtime';
import App from '../src/App';
import settingsManifest from '../../../../plugins/system-settings/manifest.json';
import '../src/style.css';

declare global {
  interface Window {
    _wails: { dispatchWailsEvent(event: { name: string; data: unknown }): void };
  }
}

const commands: { id: string; title: string; execution?: string; view?: string }[] = ['Alpha', 'Beta', 'Gamma', 'Late'].map(title => ({id:title.toLowerCase(),title}));
commands.push({id:'unregistered',title:'Unregistered',view:'external'});
commands.push({ id:'application', title:'Application', execution:'direct' });
let executions=0, dismissals=0, saves=0, clears=0, reloads=0, upgrades=0;
let installedFingerprint="b".repeat(64), sourceFingerprint="a".repeat(64);
let config={revision:0,values:{directories:[] as string[]}};
const plugin = {changed:true,id:'example.fixture',name:'Fixture',version:'0.1.0',kind:'process',state:'running',commands};
const settingsPlugin={...settingsManifest,kind:'declarative',state:'installed',commands:settingsManifest.contributes.commands};
const appPlugin={id:'system.app-search',name:'应用检索',version:'0.3.0',kind:'process',state:'installed',commands:[],settings:[{id:'directories',title:'额外应用目录',type:'directory-list'}]};
const canvas=document.createElement('canvas'); canvas.width=64; canvas.height=64;
const icon=canvas.toDataURL('image/png');
const item = (title: string) => ({icon,key:`example.fixture/demo/${title}`,pluginId:plugin.id,providerId:'demo',commandId:title.toLowerCase(),input:'fixture',title,subtitle:'Fixture',kind:'process',score:.7});
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
  mutatePackage() { sourceFingerprint="c".repeat(64); },
  counts() { return {executions,dismissals,saves,clears,config,reloads,upgrades}; },
};
Object.assign(window,{navigationFixture:fixture});
setTransport({async call(object,_method,_window,args){
  if(object!==objectNames.Call)return;
  switch(args.methodID){
    case 2914360659: case 2382139382:return {plugins:[plugin,settingsPlugin,appPlugin],diagnostics:[]};
    case 1864886877:return '';
    case 1083380061:
      sequence=String(args.args[0]);items=args.args[1]==='apps'?[item('Application')]:['Alpha','Beta','Gamma'].map(item);done=false;
      if(args.args[1]==='unknown-view')items=[item('Unregistered')];
      if(args.args[1]==='settings')items=[{...item('Settings'),key:'system.settings/$commands/open',pluginId:'system.settings',commandId:'open',title:'设置',kind:'declarative'}];
      queueMicrotask(publish);return;
    case 2263823638:
      executions++;
      return args.args[1]==='application'?{text:'',dismiss:true}:{text:args.args[2]};
    case 4294891803:dismissals++;return;
    case 2611427045:return config;
    case 2776673060:
      if(args.args[1]!==config.revision)throw new Error('设置已变化，请重新打开设置后重试');
      if(args.args[2].directories.some((path:string)=>!path.startsWith('/')))throw new Error('请输入有效的目录绝对路径');
      config={revision:config.revision+1,values:{directories:[...new Set<string>(args.args[2].directories)]}};saves++;return config;
    case 2575679692:clears++;return;
    case 1301431035:return {id:plugin.id,name:plugin.name,version:'0.2.0',kind:'process',installedVersion:plugin.version,fingerprint:sourceFingerprint,installedFingerprint,changed:plugin.version!=='0.2.0'};
    case 1907720315:
      if(!args.args[1])throw new Error('需要确认信任');
      if(args.args[2]!==sourceFingerprint||args.args[3]!==installedFingerprint)throw new Error('插件包已变化，请重新检查后再升级');
      plugin.version='0.2.0';plugin.changed=false;installedFingerprint=sourceFingerprint;upgrades++;return plugin;
    case 3444025306:plugin.changed=false;reloads++;return plugin;
    default:return;
  }
}});
createRoot(document.getElementById('root')!).render(<React.StrictMode><App /></React.StrictMode>);
