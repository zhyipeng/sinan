const send = message => window.webkit.messageHandlers.sinan.postMessage(message);
window.runProbe = async target => {
  const report = {
    bindingsAbsent: typeof window._wails === 'undefined' && typeof window.__WAILS__ === 'undefined',
    storageIsolated: false,
    ownResourceLoaded: false,
    networkBlocked: false,
  };
  // Own-origin storage may work. It must start fresh in a nonpersistent store;
  // deliberately leave the marker so another process/run detects persistence.
  try { report.storageIsolated = localStorage.getItem('probe') === null; localStorage.setItem('probe', 'fixture'); }
  catch { report.storageIsolated = true; }
  const image = url => new Promise(resolve => {
    const image = new Image();
    image.onload = () => resolve(image.naturalWidth === 1);
    image.onerror = () => resolve(false);
    image.src = url;
  });
  report.ownResourceLoaded = await image('sinan-plugin://example.ui-probe/testdata/pixel.png');
  await image('sinan-plugin://example.other/testdata/pixel.png');
  try { await fetch(target); } catch { report.networkBlocked = true; }
  send({kind:'execute', command:'unregistered'});
  send({kind:'ready', version:1}); // Replay is invalid.
  send({kind:'close', extra:true});
  // Both attempts must leave this original document alive. The native delegate
  // rejects navigation before a request can reach the local positive-control server.
  location.href = target + '?navigation';
  const link = document.createElement('a');
  link.href = target + '?popup'; link.target = '_blank';
  document.body.append(link); link.click();
  setTimeout(() => {
    send({kind:'report', report});
    if (document.body.dataset.flood === 'true') {
      for (let i=0;i<64;i++) send({kind:'unknown'});
    }
    if (document.body.dataset.revoke === 'true') send({kind:'revoke'});
    send({kind:'close'});
  }, 150);
};
send({kind:'ready', version:1});
