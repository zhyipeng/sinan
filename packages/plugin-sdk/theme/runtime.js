// Passive theme data only. This does not provide host or Wails capabilities.
(function(scope){
 const keys=['background','surface','text','muted','placeholder','line','accent','accentHover','onAccent','hover','selected','error','errorBackground','danger','warning','warningBackground','warningLine','outputLine'];
 const media=scope.matchMedia('(prefers-color-scheme: dark)');
 let state;
 function render(){
  if(!state||!scope.document.documentElement)return;
  const appearance=state.mode==='system'?(media.matches?'dark':'light'):state.mode;
  const definition=state[appearance],root=scope.document.documentElement;
  for(const key of keys){root.style.setProperty('--sinan-'+key.replace(/[A-Z]/g,c=>'-'+c.toLowerCase()),definition.tokens[key]);}
  root.style.colorScheme=appearance;root.dataset.theme=appearance;
  const current={mode:state.mode,revision:state.revision,id:definition.id,appearance,tokens:{...definition.tokens}};
  scope.__sinanTheme=Object.freeze(current);
  scope.dispatchEvent(new CustomEvent('sinan:theme-changed',{detail:current}));
 }
 scope.__sinanApplyTheme=function(next){
  if(!next||!['system','light','dark'].includes(next.mode)||!Number.isSafeInteger(next.revision)||next.revision<0||state&&next.revision<state.revision)return;
  for(const appearance of ['light','dark']){
   const def=next[appearance];if(!def||def.version!=='1'||def.appearance!==appearance||!keys.every(key=>/^#[0-9a-fA-F]{6}$/.test(def.tokens?.[key])))return;
  }
  if(state&&JSON.stringify(state)===JSON.stringify(next))return;
  state=JSON.parse(JSON.stringify(next));render();
 };
 media.addEventListener('change',()=>{if(state?.mode==='system')render();});
 scope.document.addEventListener('DOMContentLoaded',render,{once:true});
})(window);
