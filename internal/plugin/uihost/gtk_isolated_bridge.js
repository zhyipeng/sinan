// Only injected into the top frame of this session's single initial document.
// The native ports are registered in this world, never in the page world.
(function(scope, entry) {
  if (scope.top !== scope || scope.location.href !== entry) return;
  const ports = scope.webkit.messageHandlers;
  let pending = false;
  function reply(id, result, error) {
    scope.postMessage({channel:'sinan:reply', id, result, error}, '*');
  }
  scope.addEventListener('message', event => {
    if (!event.isTrusted || event.source !== scope || scope.location.href !== entry) return;
    const data = event.data;
    if (!data || data.channel !== 'sinan:request' || !Number.isSafeInteger(data.id) || data.id <= 0) return;
    if (data.kind === 'message') {
      ports.sinan.postMessage(data.request);
      return;
    }
    if (data.kind !== 'call') return;
    if (pending) { reply(data.id, undefined, '已有 UI 调用正在处理'); return; }
    pending = true;
    Promise.resolve().then(() => ports.sinanCall.postMessage(data.request)).then(
      result => reply(data.id, result, undefined),
      () => reply(data.id, undefined, 'UI 调用失败或会话已关闭')
    ).finally(() => { pending = false; });
  });
})(window, __SINAN_ENTRY__);
