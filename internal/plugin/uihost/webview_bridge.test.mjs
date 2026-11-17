import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import test from 'node:test';
import vm from 'node:vm';

const entry='https://test.sinan.invalid/index.html';
const script=readFileSync(new URL('webview_page_bridge.js',import.meta.url),'utf8').replaceAll('__SINAN_ENTRY__',JSON.stringify(entry));
function fixture({top=true,url=entry,sendError=false}={}) {
  const delays=[], sent=[], listeners=[], timers=new Map(); let timerID=0;
  const page={location:{href:url}, chrome:{webview:{postMessage(data){if(sendError)throw new Error('internal diagnostic');sent.push(structuredClone(data));},addEventListener(name,listener){if(name==='message')listeners.push(listener);}}}};
  page.Document=class { execCommand(command) { return command==='selectAll'; } };
  page.top=top?page:{};
  vm.runInNewContext(script,{window:page,setTimeout(fn,delay){delays.push(delay);const id=++timerID;timers.set(id,fn);return id;},clearTimeout(id){timers.delete(id);}});
  return {page,sent,timers,delays,reply(data){for(const listener of listeners)listener({data});},expire(){for(const fn of [...timers.values()])fn();timers.clear();}};
}

test('WebView2 facade keeps the existing narrow SDK shape',async()=>{
  const f=fixture();
  assert.equal(Object.isFrozen(f.page.webkit.messageHandlers),true);
  assert.equal(Object.isFrozen(f.page.webkit.messageHandlers.sinanCall),true);
  assert.equal(Object.getOwnPropertyDescriptor(f.page,'webkit').configurable,false);
  assert.equal(f.page._wails,undefined);
  assert.equal(f.page.runtime,undefined);
  f.page.webkit.messageHandlers.sinan.postMessage({kind:'ready',version:2});
  const pending=f.page.webkit.messageHandlers.sinanCall.postMessage({method:'launch.get'});
  assert.deepEqual(f.sent,[{channel:'sinan:request',kind:'message',id:1,request:{kind:'ready',version:2}},{channel:'sinan:request',kind:'call',id:2,request:{method:'launch.get'}}]);
  f.reply({channel:'sinan:reply',id:2,result:{input:'kept'}});
  assert.equal((await pending).input,'kept');
  assert.equal(f.timers.size,0);
});

test('only matching host reply IDs settle calls',async()=>{
  const f=fixture();let settled=false;
  const pending=f.page.webkit.messageHandlers.sinanCall.postMessage({method:'storage.get'}).then(value=>{settled=true;return value;});
  f.reply({channel:'other',id:1,result:'wrong'});
  f.reply({channel:'sinan:reply',id:2,result:'wrong'});
  await Promise.resolve();assert.equal(settled,false);
  f.reply({channel:'sinan:reply',id:1,result:'correct'});
  assert.equal(await pending,'correct');
  f.reply({channel:'sinan:reply',id:1,result:'duplicate'});
  assert.equal(f.timers.size,0);
});

test('timeout and send failure release pending slots with a fixed error',async()=>{
  const f=fixture();const calls=[];
  for(let i=0;i<8;i++)calls.push(f.page.webkit.messageHandlers.sinanCall.postMessage({method:'storage.get'}));
  await assert.rejects(f.page.webkit.messageHandlers.sinanCall.postMessage({method:'storage.get'}),/调用过于频繁/);
  const rejected=calls.map(call=>assert.rejects(call,/UI 调用失败或会话已关闭/));
  f.expire();await Promise.all(rejected);
  const next=f.page.webkit.messageHandlers.sinanCall.postMessage({method:'storage.get'});
  f.reply({channel:'sinan:reply',id:9,error:'UI 调用失败或会话已关闭'});
  await assert.rejects(next,/UI 调用失败或会话已关闭/);
  const broken=fixture({sendError:true});
  for(let i=0;i<12;i++)await assert.rejects(broken.page.webkit.messageHandlers.sinanCall.postMessage({method:'storage.get'}),/UI 调用失败或会话已关闭/);
  assert.equal(broken.timers.size,0);
});

test('foreign documents and frames get no compatibility bridge',()=>{
  for(const options of [{top:false},{url:'about:blank'},{url:entry+'#part'}]){
    const f=fixture(options);
    assert.equal(f.page.webkit,undefined);
    assert.equal(typeof f.page.print,'function');
    assert.equal(f.page.RTCPeerConnection,undefined);
    assert.equal(Object.getOwnPropertyDescriptor(f.page,'RTCPeerConnection').configurable,false);
    for (const key of ['showOpenFilePicker','showSaveFilePicker','showDirectoryPicker']) {
      assert.equal(f.page[key],undefined);
      assert.equal(Object.getOwnPropertyDescriptor(f.page,key).configurable,false);
    }
    const document=new f.page.Document();
    assert.equal(document.execCommand('PRINT'),false);
    assert.equal(document.execCommand('SaveAs'),false);
    assert.equal(document.execCommand('selectAll'),true);
    assert.equal(Object.getOwnPropertyDescriptor(f.page.Document.prototype,'execCommand').configurable,false);
    f.page.print();assert.equal(f.sent.length,0);
  }
});

test('window execution and native PNG save receive bounded long deadlines',async()=>{
 const f=fixture();const long=f.page.webkit.messageHandlers.sinanCall.postMessage({method:'window.execute',commandId:'region'});assert.equal(f.delays.at(-1),127000);f.reply({channel:'sinan:reply',id:1,result:{text:'done'}});await long;
 const save=f.page.webkit.messageHandlers.sinanCall.postMessage({method:'file.savePNG',commandId:'save'});assert.equal(f.delays.at(-1),127000);f.reply({channel:'sinan:reply',id:2,result:{saved:false}});await save;
 const ordinary=f.page.webkit.messageHandlers.sinanCall.postMessage({method:'command.execute',commandId:'region'});assert.equal(f.delays.at(-1),2000);f.reply({channel:'sinan:reply',id:3,result:{text:'done'}});await ordinary;
});

test('file.openImage receives the dialog deadline',async()=>{
 const f=fixture();const opened=f.page.webkit.messageHandlers.sinanCall.postMessage({method:'file.openImage'});assert.equal(f.delays.at(-1),127000);f.reply({channel:'sinan:reply',id:1,result:null});assert.equal(await opened,null);
});
