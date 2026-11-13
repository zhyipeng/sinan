import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import vm from 'node:vm';

const entry = 'sinan-plugin://example.bridge/index.html';
const load = file => readFileSync(new URL(file, import.meta.url), 'utf8').replaceAll('__SINAN_ENTRY__', JSON.stringify(entry));
const settle = () => new Promise(resolve => setImmediate(resolve));

function fixture({top = true, url = entry, execute = async () => ({ok:true})} = {}) {
  const listeners = new Map();
  const page = {location:{href:url}};
  const isolated = {location:{href:url}};
  const lifecycle = [], calls = [];
  for (const scope of [page, isolated]) {
    scope.top = top ? scope : {};
    listeners.set(scope, []);
    scope.addEventListener = (kind, listener) => { if (kind === 'message') listeners.get(scope).push(listener); };
    scope.postMessage = data => queueMicrotask(() => {
      for (const target of [page, isolated]) for (const listener of listeners.get(target)) listener({isTrusted:true, source:target, data:structuredClone(data)});
    });
  }
  isolated.webkit = {messageHandlers:{
    sinan:{postMessage(request) { lifecycle.push(request); }},
    sinanCall:{postMessage(request) { calls.push(request); return execute(request); }},
  }};
  vm.runInNewContext(load('gtk_isolated_bridge.js'), {window:isolated});
  vm.runInNewContext(load('gtk_page_bridge.js'), {window:page});
  return {page, isolated, lifecycle, calls, emit(event) { for (const listener of listeners.get(isolated)) listener(event); }};
}

test('existing WebKit SDK shape reaches only the narrow native ports', async () => {
  const f = fixture();
  f.page.webkit.messageHandlers.sinan.postMessage({kind:'ready', version:2});
  const value = await f.page.webkit.messageHandlers.sinanCall.postMessage({method:'storage.get'});
  assert.equal(JSON.stringify(value), '{"ok":true}');
  assert.equal(JSON.stringify(f.lifecycle), '[{"kind":"ready","version":2}]');
  assert.equal(JSON.stringify(f.calls), '[{"method":"storage.get"}]');
  assert.equal(f.page._wails, undefined);
  assert.equal(f.page.runtime, undefined);
});

test('foreign frame and synthetic events cannot reach native ports', async () => {
  const f = fixture();
  const data = {channel:'sinan:request', kind:'call', id:1, request:{method:'storage.get'}};
  f.emit({isTrusted:false, source:f.isolated, data});
  f.emit({isTrusted:true, source:{}, data});
  f.emit({isTrusted:true, source:f.isolated, data:{...data, id:-1}});
  f.emit({isTrusted:true, source:f.isolated, data:{...data, channel:'other'}});
  await settle();
  assert.equal(f.calls.length, 0);
  f.isolated.location.href = 'about:blank';
  f.emit({isTrusted:true, source:f.isolated, data});
  await settle();
  assert.equal(f.calls.length, 0);
});

test('bridge is not installed in subframes or a different document', () => {
  for (const options of [{top:false}, {url:'about:blank'}]) {
    const f = fixture(options);
    assert.equal(f.page.webkit, undefined);
  }
});

test('native calls are single-flight and failure releases the slot', async () => {
  let release;
  let count = 0;
  const f = fixture({execute() { count++; return new Promise(resolve => { release = resolve; }); }});
  const first = f.page.webkit.messageHandlers.sinanCall.postMessage({method:'storage.get'});
  await settle();
  await assert.rejects(f.page.webkit.messageHandlers.sinanCall.postMessage({method:'storage.get'}));
  assert.equal(count, 1);
  release({revision:1});
  await first;
  await settle();
  const next = f.page.webkit.messageHandlers.sinanCall.postMessage({method:'storage.get'});
  await settle();
  assert.equal(count, 2);
  release({revision:2});
  await next;
});

test('native rejection returns a fixed error without forwarding arbitrary details', async () => {
  const f = fixture({execute:async () => { throw new Error('synthetic internal diagnostic'); }});
  await assert.rejects(f.page.webkit.messageHandlers.sinanCall.postMessage({method:'storage.get'}), /UI 调用失败或会话已关闭/);
});
