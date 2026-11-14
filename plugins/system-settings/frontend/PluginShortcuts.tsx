import {useEffect,useState,useMemo} from 'react';
import Shortcut,{type ShortcutBridge,type ShortcutState} from './Shortcut';

export type PluginShortcut = {pluginId:string;commandId:string;pluginName:string;title:string;state:ShortcutState};
export type PluginShortcutsBridge = {
 list():Promise<PluginShortcut[]>;
 forCommand(pluginId:string,commandId:string):ShortcutBridge;
 subscribe(changed:()=>void):()=>void;
};
function PluginShortcutRow({item,bridge}:{item:PluginShortcut;bridge:PluginShortcutsBridge}){
 const commandBridge=useMemo(()=>bridge.forCommand(item.pluginId,item.commandId),[bridge,item.pluginId,item.commandId]);
 return <Shortcut bridge={commandBridge} title={item.title} />;
}
export default function PluginShortcuts({bridge}:{bridge:PluginShortcutsBridge}){
 const [items,setItems]=useState<PluginShortcut[]>([]),[error,setError]=useState('');
 useEffect(()=>{let active=true;const load=()=>{void bridge.list().then(rows=>{if(active){setItems(rows);setError('');}}).catch(()=>{if(active)setError('无法读取插件快捷键，请重新打开设置。');});};const off=bridge.subscribe(load);load();return()=>{active=false;off();};},[bridge]);
 if(!items.length&&!error)return null;
 return <section aria-label="插件快捷键"><h2>插件快捷键</h2>{items.map(item=><PluginShortcutRow key={`${item.pluginId}/${item.commandId}`} item={item} bridge={bridge}/>)}{error&&<p className="error" role="alert">{error}</p>}</section>;
}
