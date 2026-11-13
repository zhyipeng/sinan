// This runs in an isolated WebView2, never in the trusted Wails document.
(function(scope, entry) {
  // Prevent engine UI even in an unexpected subframe. Native policy separately
  // denies frame navigation, permissions, browser accelerators and file pickers.
  Object.defineProperty(scope, 'print', {value() {}, writable:false, configurable:false});
  for (const key of ['RTCPeerConnection', 'webkitRTCPeerConnection', 'showOpenFilePicker', 'showSaveFilePicker', 'showDirectoryPicker']) {
    Object.defineProperty(scope, key, {value:undefined, writable:false, configurable:false});
  }
  const documentPrototype = scope.Document?.prototype;
  if (documentPrototype?.execCommand) {
    const execCommand = documentPrototype.execCommand;
    Object.defineProperty(documentPrototype, 'execCommand', {value(command, ...args) {
      if (['print', 'saveas'].includes(String(command).toLowerCase())) return false;
      return Reflect.apply(execCommand, this, [command, ...args]);
    }, writable:false, configurable:false});
  }
  if (scope.top !== scope || scope.location.href !== entry) return;
  const port = scope.chrome.webview;
  const send = port.postMessage.bind(port);
  let sequence = 0;
  const pending = new Map();
  port.addEventListener('message', event => {
    const reply = event.data;
    if (reply?.channel !== 'sinan:reply') return;
    const waiter = pending.get(reply.id);
    if (!waiter) return;
    pending.delete(reply.id);
    clearTimeout(waiter.timer);
    if (typeof reply.error === 'string') waiter.reject(new Error(reply.error));
    else waiter.resolve(reply.result);
  });
  const sinan = Object.freeze({postMessage(request) {
    send({channel:'sinan:request', kind:'message', id:++sequence, request});
  }});
  const sinanCall = Object.freeze({postMessage(request) {
    if (pending.size >= 8) return Promise.reject(new Error('调用过于频繁，请稍后重试。'));
    return new Promise((resolve, reject) => {
      const id = ++sequence;
      const timer = setTimeout(() => {
        pending.delete(id);
        reject(new Error('UI 调用失败或会话已关闭'));
      }, 2000);
      pending.set(id, {resolve, reject, timer});
      try { send({channel:'sinan:request', kind:'call', id, request}); }
      catch (_) {
        clearTimeout(timer); pending.delete(id);
        reject(new Error('UI 调用失败或会话已关闭'));
      }
    });
  }});
  Object.defineProperty(scope, 'webkit', {value:Object.freeze({messageHandlers:Object.freeze({sinan, sinanCall})}), configurable:false, writable:false});
})(window, __SINAN_ENTRY__);
