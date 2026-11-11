import {Events} from '@wailsio/runtime';
import {ApplyThemeAppearance,ThemeSettings} from '../bindings/sinan/apps/desktop/hostservice';
import '../../../../packages/plugin-sdk/theme/runtime.js';
export function startTheme(){
 const receive=(state:unknown)=>window.__sinanApplyTheme(state);
 Events.On('theme:changed',event=>receive(event.data));
 let sequence=0;
 window.addEventListener('sinan:theme-changed',()=>{
  const current=window.__sinanTheme;if(!current)return;
  const request=++sequence;
  // Appearance messages affect this real window only, never search or form state.
  void ApplyThemeAppearance(current.revision,current.appearance).catch(()=>{if(request===sequence)void ThemeSettings().then(latest=>{if(request===sequence&&latest.revision>current.revision)receive(latest);}).catch(()=>{});});
 });
 void ThemeSettings().then(receive).catch(()=>{});
}
declare global{
 interface Window{
  __sinanApplyTheme(state:unknown):void;
  __sinanTheme?:{mode:'system'|'light'|'dark';revision:number;appearance:'light'|'dark';id:string;tokens:Record<string,string>};
 }
}
