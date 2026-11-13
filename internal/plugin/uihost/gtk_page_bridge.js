// SDK compatibility facade in the page world. It carries no native capability.
(function(scope, entry) {
  if (scope.top !== scope || scope.location.href !== entry) return;
  let sequence = 0;
  const pending = new Map();
  function post(kind, request) {
    const id = ++sequence;
    scope.postMessage({channel:'sinan:request', kind, id, request}, '*');
    return id;
  }
  scope.addEventListener('message', event => {
    if (!event.isTrusted || event.source !== scope || event.data?.channel !== 'sinan:reply') return;
    const reply = event.data;
    const waiter = pending.get(reply.id);
    if (!waiter) return;
    pending.delete(reply.id);
    if (typeof reply.error === 'string') waiter.reject(new Error(reply.error));
    else waiter.resolve(reply.result);
  });
  const sinan = Object.freeze({postMessage(request) { post('message', request); }});
  const sinanCall = Object.freeze({postMessage(request) {
    if (pending.size >= 8) return Promise.reject(new Error('调用过于频繁，请稍后重试。'));
    return new Promise((resolve, reject) => {
      const id = ++sequence;
      pending.set(id, {resolve, reject});
      scope.postMessage({channel:'sinan:request', kind:'call', id, request}, '*');
    });
  }});
  // Real native ports exist only in a separate, host-injected script world.
  Object.defineProperty(scope, 'webkit', {value:Object.freeze({messageHandlers:Object.freeze({sinan, sinanCall})}), configurable:false, writable:false});
})(window, __SINAN_ENTRY__);
