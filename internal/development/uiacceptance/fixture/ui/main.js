const text = document.querySelector('#text');
const composition = document.querySelector('#composition');
const count = document.querySelector('#count');
const status = document.querySelector('#status');
const hint = document.querySelector('#hint');
const bridge = window.webkit?.messageHandlers?.sinan;
const send = message => bridge?.postMessage(message);
let composing = false;
let armed = false;
const hang = document.querySelector('#hang');
function update() {
  composition.textContent = composing ? '正在组词' : document.activeElement === text ? '可以输入' : '已失焦';
  count.textContent = `${Array.from(text.value).length} 字符`;
}
text.addEventListener('compositionstart', () => { composing = true; update(); });
text.addEventListener('compositionend', () => { composing = false; update(); });
for (const event of ['input', 'focus', 'blur']) text.addEventListener(event, update);
document.querySelector('#clear').addEventListener('click', () => { text.value = ''; text.focus(); update(); });
document.querySelector('#close').addEventListener('click', () => send({kind:'close'}));
hang.addEventListener('click', () => {
  if (!armed) {
    armed = true;
    hang.textContent = '再次点击，开始卡死';
    hint.textContent = '窗口应被回收，司南仍可呼出并重新打开窗口。';
    return;
  }
  // Deliberate, isolated failure. Browser preview never enters the loop.
  if (!bridge) { hint.textContent = '浏览器预览不执行卡死；请使用原生验收窗口。'; return; }
  while (true) {}
});
if (bridge) { send({kind:'ready',version:1}); status.textContent = '已连接'; }
else { status.textContent = '页面预览'; document.querySelector('#close').disabled = true; }
text.focus();
update();
