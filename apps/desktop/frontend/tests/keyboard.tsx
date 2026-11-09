// Deterministic browser fixture: real App and Wails event delivery, mocked IPC.
// Providers advance only when the test asks, so slow-machine timing is irrelevant.
import React from 'react';
import { createRoot } from 'react-dom/client';
import { objectNames, setTransport } from '@wailsio/runtime';
import App from '../src/App';
import settingsManifest from '../../../../plugins/system-settings/manifest.json';
import managerManifest from '../../../../plugins/system-plugin-manager/manifest.json';
import '../src/style.css';

declare global {
  interface Window {
    _wails: { dispatchWailsEvent(event: { name: string; data: unknown }): void };
  }
}

const commands: { id: string; title: string; execution?: string; uiView?:string; view?: string }[] = ['Alpha', 'Beta', 'Gamma', 'Late'].map(title => ({id:title.toLowerCase(),title}));
commands.push({id:'unregistered',title:'Unregistered',view:'external'});
commands.push({id:'window',title:'Independent window',uiView:'main'});
commands.push({ id:'application', title:'Application', execution:'direct' });
let executions=0, dismissals=0, saves=0, clears=0, reloads=0, upgrades=0, detaches=0;
let installedFingerprint="b".repeat(64), sourceFingerprint="a".repeat(64);
let config={revision:0,values:{directories:[] as string[]}};
let shortcut={shortcut:'Cmd+Space',active:'Cmd+Space',default:'Cmd+Space',revision:0,error:'',ready:true};
let detachShortcut={shortcut:'Shift+Cmd+D',active:'Shift+Cmd+D',default:'Shift+Cmd+D',revision:0,error:'',ready:true};
let recording=false;
let managerPresent=true, fixturePresent=true, managerRestores=0, stops=0, uninstalls=0;
let uiEnabled=false, openedViews=0, commandWindows=0, failWindowOpen=false, deferWindow=false;
let completeWindow: (()=>void)|undefined;
let commandOpenArgs:unknown[]=[];
let brokenName=false;
let failEnablement=false;
const plugin = {uiViews:[{id:'main',title:'',entry:'index.html',bridgeVersion:'1'},{id:'a'.repeat(64),entry:'other.html',bridgeVersion:'1'}],disabled:false,changed:true,id:'example.fixture',name:'Fixture',version:'0.1.0',kind:'process',state:'running',commands};
const settingsPlugin={protected:true,disabled:false,...settingsManifest,kind:'declarative',state:'installed',commands:settingsManifest.contributes.commands};
const managerPlugin={protected:true,disabled:false,...managerManifest,kind:'declarative',state:'installed',commands:managerManifest.contributes.commands};
const appPlugin={disabled:false,id:'system.app-search',name:'应用检索',version:'0.4.0',kind:'process',state:'installed',commands:[],settings:[{id:'directories',title:'额外应用目录',type:'directory-list'}]};
const canvas=document.createElement('canvas'); canvas.width=64; canvas.height=64;
const icon=canvas.toDataURL('image/png');
const item = (title: string) => ({icon,key:`example.fixture/demo/${title}`,pluginId:plugin.id,providerId:'demo',commandId:title.toLowerCase(),input:'fixture',title,subtitle:'Fixture',kind:'process',score:.7});
let sequence = '';
let items = ['Alpha','Beta','Gamma'].map(item);
let done = false;
function publish() {
  window._wails.dispatchWailsEvent({name:'search:update',data:{queryId:sequence,items:[...items],pending:done?0:1,failures:[],done}});
}
let permissionSaves=0;
let permissionState={revision:0,error:'',plugins:[{id:plugin.id,name:plugin.name,fingerprint:'1'.repeat(64),reviewed:false,stale:false,changed:false,permissions:[{id:'main/command/alpha',title:'运行命令：Alpha',allowed:false},{id:'main/storage',title:'读写私有数据',allowed:false}]}]};
let dataClears=0;
let dataRows=[{id:plugin.id,name:'Fixture',installed:true,bytes:128,corrupt:false,token:'f'.repeat(64)},{id:'example.orphan',name:'example.orphan',installed:false,bytes:17,corrupt:true,token:'e'.repeat(64)}];
const fixture = {
  stalePermissionRevision(){permissionState.revision++;},
  mutatePermissionPackage(){permissionState.plugins[0]={...permissionState.plugins[0],fingerprint:"2".repeat(64),reviewed:false,stale:true,permissions:permissionState.plugins[0].permissions.map(cap=>({...cap,allowed:false}))};},
  mutateData(){dataRows[0].token='a'.repeat(64);},
  addUnclearableData(){dataRows.push({id:'example.link',name:'example.link',installed:false,bytes:0,corrupt:true,token:''});},
  advance() { items = [item('Late'),...items.filter(item=>item.title!=='Late')]; publish(); },
  complete() { done=true; publish(); },
  refresh() { publish(); },
  mutatePackage() { sourceFingerprint="c".repeat(64); },
  counts() { return {permissionSaves,dataClears,detachShortcut,detaches,pendingWindow:!!completeWindow,commandWindows,commandOpenArgs,openedViews,executions,dismissals,saves,clears,config,reloads,upgrades,shortcut,recording,managerPresent,managerRestores,stops,uninstalls,fixtureDisabled:plugin.disabled,managerDisabled:managerPlugin.disabled}; },
  enableUI(){uiEnabled=true;},
  nameView(){plugin.uiViews[0].title='输入视图';},
  readyWindows(){uiEnabled=true;plugin.changed=false;},
  failNextWindowOpen(){failWindowOpen=true;},
  deferWindowOpen(){deferWindow=true;},
  completeWindowOpen(){completeWindow?.();},
  showLauncher(){window._wails.dispatchWailsEvent({name:'launcher:show',data:null});},
  failWindow(){window._wails.dispatchWailsEvent({name:'ui:closed',data:{pluginId:plugin.id,viewId:'main',failed:true}});},
  // Simulate external damage/legacy policy, never an allowed management action.
  disableManagerExternally() {managerPlugin.disabled=true;},
  removeManagerExternally() {managerPresent=false;},
  failNextToggle() {failEnablement=true;},
  breakManager() { brokenName=true; },
  repairFixture() {brokenName=false;},
  captureShortcut() { if(recording)window._wails.dispatchWailsEvent({name:'shortcut:recorded',data:shortcut.active}); },
};
Object.assign(window,{navigationFixture:fixture});
setTransport({async call(object,_method,_window,args){
  if(object!==objectNames.Call)return;
  switch(args.methodID){
    case 1580998567:return structuredClone(permissionState);
    case 2058116116:{
      const target=permissionState.plugins[0];
      if(args.args[0]!==target.id||args.args[1]!==target.fingerprint)throw new Error('插件包已变化，请刷新权限后重新确认');
      if(args.args[2]!==permissionState.revision)throw new Error('权限设置已变化，请刷新后重试');
      if(args.args[3].some((id:string)=>!target.permissions.some(cap=>cap.id===id)))throw new Error('权限未声明');
      permissionState={...permissionState,revision:permissionState.revision+1,plugins:[{...target,reviewed:true,stale:false,permissions:target.permissions.map(cap=>({...cap,allowed:args.args[3].includes(cap.id)}))}]};
      permissionSaves++;return structuredClone(permissionState);
    }
    case 3637179584:return dataRows.map(row=>({...row,installed:row.id===plugin.id?fixturePresent:row.installed}));
    case 1111664535:{
      const row=dataRows.find(row=>row.id===args.args[0]);
      if(!row||!row.token||row.token!==args.args[1])throw new Error('插件数据已变化，请刷新后重新确认');
      dataRows=dataRows.filter(candidate=>candidate!==row);dataClears++;return;
    }
    case 4262640395:return detachShortcut;
    case 1332948311:
      if(args.args[0]===shortcut.shortcut)throw new Error('呼出与窗口独立快捷键不能相同');
      if(args.args[1]!==detachShortcut.revision)throw new Error('快捷键已变化');
      detachShortcut={...detachShortcut,shortcut:args.args[0],active:args.args[0],revision:detachShortcut.revision+1};
      queueMicrotask(()=>window._wails.dispatchWailsEvent({name:'detach-shortcut:changed',data:detachShortcut}));
      return detachShortcut;
    case 3741028594:
      detaches++;
      window._wails.dispatchWailsEvent({name:'view:detached',data:null});
      return;
    case 2354522669:
      if(!uiEnabled)throw new Error('独立插件窗口尚未启用');
      if(plugin.disabled||plugin.changed||args.args[0]!==plugin.id||args.args[1]!=='window')throw new Error('视图不可用');
      commandOpenArgs=[...args.args];
      if(failWindowOpen){failWindowOpen=false;throw new Error('UI 客体启动或握手失败');}
      if(deferWindow){deferWindow=false;return new Promise(resolve=>{completeWindow=()=>{commandWindows++;resolve(undefined);};});}
      commandWindows++;return;
    case 1572518711:return uiEnabled;
    case 188677181:
      if(!uiEnabled||plugin.disabled||plugin.changed||args.args[0]!==plugin.id||args.args[1]!=='main')throw new Error('视图不可用');
      openedViews++;return;
    case 2914360659: case 2382139382:return {plugins:[...(fixturePresent?[{...plugin,name:brokenName?{invalid:true}:plugin.name}]:[]),settingsPlugin,appPlugin,...(managerPresent?[managerPlugin]:[])],diagnostics:[]};
    case 1864886877:return '';
    case 3261602732:return shortcut;
    case 851920844:recording=Boolean(args.args[0]);return;
    case 2913582280:
      if(args.args[0]===detachShortcut.shortcut)throw new Error('呼出与窗口独立快捷键不能相同');
      if(args.args[1]!==shortcut.revision)throw new Error('快捷键设置已变化');
      if(args.args[0]==='Ctrl+Alt+K')throw new Error('快捷键可能已被系统或其他应用占用；原快捷键保持不变。');
      shortcut={...shortcut,shortcut:args.args[0],active:args.args[0],revision:shortcut.revision+1};
      queueMicrotask(()=>window._wails.dispatchWailsEvent({name:'shortcut:changed',data:shortcut}));return shortcut;
    case 1083380061:
      sequence=String(args.args[0]);items=args.args[1]==='apps'?[item('Application')]:['Alpha','Beta','Gamma'].map(item);done=false;
      if(args.args[1]==='window')items=[{...item('Window'),commandId:'window',providerId:'$commands',title:'独立窗口'}];
      if(args.args[1]==='unknown-view')items=[item('Unregistered')];
      if(args.args[1]==='settings')items=[{...item('Settings'),key:'system.settings/$commands/open',pluginId:'system.settings',commandId:'open',title:'设置',kind:'declarative'}];
      if(args.args[1]==='plugins')items=managerPresent?[{...item('Manager'),key:'system.plugin-manager/$commands/open',pluginId:managerManifest.id,commandId:'open',title:'插件管理',kind:'declarative'}]:[];
      items=items.filter(item=>![plugin,settingsPlugin,managerPlugin,appPlugin].find(plugin=>plugin.id===item.pluginId)?.disabled);
      if(items.length===0)done=true;
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
    case 110346107:plugin.state='deactivated';stops++;return;
    case 448181795: {
      if([settingsPlugin.id,managerPlugin.id].includes(args.args[0])&&!args.args[1])throw new Error("系统插件不允许禁用");
      if(failEnablement){failEnablement=false;throw new Error('设置保存失败，请检查本地存储后重试');}
      const target=[plugin,settingsPlugin,managerPlugin,appPlugin].find(plugin=>plugin.id===args.args[0]);
      if(!target)throw new Error('插件不存在');
      target.disabled=!args.args[1];target.state=target.disabled?'disabled':'installed';return target;
    }
    case 147137039:
      if([settingsPlugin.id,managerPlugin.id].includes(args.args[0]))throw new Error("系统插件不允许卸载");
      if(args.args[0]===managerManifest.id)managerPresent=false;else fixturePresent=false;
      uninstalls++;return;
    case 599706841:
      if(args.args[0]!=='/tmp/sinan-manager-package')throw new Error('仅支持设置或插件管理的声明式组件包');
      return {id:managerManifest.id,name:managerManifest.name,version:'0.1.0',kind:'declarative',installedVersion:managerPresent?'0.1.0':'',fingerprint:'d'.repeat(64),installedFingerprint:managerPresent?'e'.repeat(64):'',changed:true};
    case 3812136400:
      if(args.args[0]!=='/tmp/sinan-manager-package'||args.args[1]!=='d'.repeat(64)||args.args[2]!== (managerPresent?'e'.repeat(64):''))throw new Error('插件文件已变化，请重新检查后修复');
      managerPresent=true;managerRestores++;return managerPlugin;
    case 1301431035:
      if(args.args[0]==='/tmp/sinan-manager-package')return {id:managerManifest.id,name:managerManifest.name,version:'0.1.0',kind:'declarative',installedVersion:managerPresent?'0.1.0':'',fingerprint:'d'.repeat(64),installedFingerprint:managerPresent?'e'.repeat(64):'',changed:true};
      return {id:plugin.id,name:plugin.name,version:'0.2.0',kind:'process',installedVersion:plugin.version,fingerprint:sourceFingerprint,installedFingerprint,changed:plugin.version!=='0.2.0'};
    case 1907720315:
      if(args.args[0]==='/tmp/sinan-manager-package'){managerPresent=true;managerRestores++;return managerPlugin;}
      if(!args.args[1])throw new Error('需要确认信任');
      if(args.args[2]!==sourceFingerprint||args.args[3]!==installedFingerprint)throw new Error('插件包已变化，请重新检查后再升级');
      plugin.version='0.2.0';plugin.changed=false;installedFingerprint=sourceFingerprint;upgrades++;return plugin;
    case 3444025306:plugin.changed=false;reloads++;return plugin;
    default:return;
  }
}});
createRoot(document.getElementById('root')!).render(<React.StrictMode><App /></React.StrictMode>);
