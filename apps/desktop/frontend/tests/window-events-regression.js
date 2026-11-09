// Run against npm run dev; real App + Wails listeners, deterministic IPC fixture.
const task = await taskSpace('Sinan detached window event isolation regression');
console.log({taskSpaceId:task.spaceId});
const page = task.page('p1');
const query = 'input[aria-label="搜索命令"]';
async function open(value) {
  await page.goto('http://127.0.0.1:9245/tests/keyboard.html');
  await page.fill(query, value);
  await page.waitForSelector('.command');
  await page.click('.command');
  await page.waitForSelector('h1');
}
await open('settings');
const field = 'textarea[id="system.app-search-directories"]';
await page.waitForFunction(()=>document.querySelector('textarea') && !document.querySelector('textarea').disabled);
await page.fill(field, '/tmp/sinan-unsaved-window-draft');
await page.keyboard.press('Meta+Shift+D');
await page.waitForSelector('.shell.detached');
await page.waitForSelector('button[aria-label="关闭窗口"]');
await page.evaluate(()=>window.navigationFixture.showLauncher());
await page.waitForFunction(()=>document.querySelector('h1')?.textContent==='设置' && document.querySelector('textarea')?.value==='/tmp/sinan-unsaved-window-draft', undefined, {timeout:2000});
if(await page.evaluate(()=>!!document.querySelector('input[aria-label="搜索命令"]')))throw new Error('detached settings reset to search');
if(await page.evaluate(()=>window.navigationFixture.counts().detaches)!==1)throw new Error('duplicate detach');
console.log('PASS: detached settings and unsaved input survive launcher reset event');

await open('plugins');
const directory = 'input[id="directory"]';
await page.waitForSelector(directory);
await page.fill(directory, '/tmp/sinan-manager-window-draft');
await page.click('button[aria-label="独立窗口"]');
await page.waitForSelector('.shell.detached');
await page.evaluate(()=>window.navigationFixture.showLauncher());
await page.waitForFunction(()=>document.querySelector('h1')?.textContent==='插件管理' && document.querySelector('input[id="directory"]')?.value==='/tmp/sinan-manager-window-draft', undefined, {timeout:2000});
console.log('PASS: detached plugin manager keeps its page and directory draft');

await page.goto('http://127.0.0.1:9245/tests/keyboard.html');
await page.fill(query,'demo');
await page.waitForSelector('.command');
await page.evaluate(()=>window.navigationFixture.showLauncher());
await page.waitForFunction(()=>document.querySelector('input[aria-label="搜索命令"]')?.value==='');
if(await page.evaluate(()=>!!document.querySelector('.shell.detached')))throw new Error('replacement main window became detached');
await page.fill(query, 'settings');
await page.waitForSelector('.command');
await page.click('.command');
await page.waitForSelector('button[aria-label="独立窗口"]');
console.log('PASS: main search reset still works and can open another detachable view');
await task.finish({keep:[]});
