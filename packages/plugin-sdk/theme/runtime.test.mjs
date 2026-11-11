import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {readFile} from 'node:fs/promises';
const source=await readFile(new URL('./runtime.js',import.meta.url),'utf8');
const [light,dark]=JSON.parse(await readFile(new URL('../../../internal/theme/builtin.json',import.meta.url),'utf8'));
function runtime(){
 const values={},listeners={};const media={matches:false,addEventListener(_,fn){this.changed=fn;}};
 const window={document:{documentElement:{style:{setProperty(key,value){values[key]=value;}},dataset:{}},addEventListener(){}},matchMedia(){return media;},dispatchEvent(event){listeners[event.type]?.(event);}};
 vm.runInNewContext(source,{window,structuredClone,CustomEvent:class{constructor(type,options){this.type=type;this.detail=options.detail;}}});
 return {window,media,values};
}
test('system changes are live, explicit mode ignores system and old snapshots',()=>{
 const {window,media,values}=runtime();const state={mode:'system',revision:0,light,dark};
 window.__sinanApplyTheme(state);assert.equal(values['--sinan-background'],light.tokens.background);
 media.matches=true;media.changed();assert.equal(window.__sinanTheme.appearance,'dark');
 window.__sinanApplyTheme({...state,mode:'light',revision:1});media.matches=false;media.changed();media.matches=true;media.changed();assert.equal(window.__sinanTheme.appearance,'light');
 window.__sinanApplyTheme({...state,mode:'dark',revision:0});assert.equal(window.__sinanTheme.appearance,'light');
 const unsafe=structuredClone(state);unsafe.revision=2;unsafe.dark.tokens.background='url(https://example.invalid)';window.__sinanApplyTheme(unsafe);assert.equal(window.__sinanTheme.revision,1);
 assert.equal(window.document.documentElement.style.colorScheme,'light');
});
test('CSS fallbacks agree with both palette definitions',async()=>{
 const css=await readFile(new URL('./builtin.css',import.meta.url),'utf8');
 for(const definition of [light,dark])for(const [key,value]of Object.entries(definition.tokens))assert.ok(css.includes('--sinan-'+key.replace(/[A-Z]/g,c=>'-'+c.toLowerCase())+':'+value+';'));
});
