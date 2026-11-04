// Run against `npm run dev`; requires the local ego-browser CLI.
const task = await taskSpace('Sinan pending-search keyboard regression');
const page = task.page('p1');
const query = 'input[aria-label="搜索命令"]';
async function selected(expected, label) {
  const actual = await page.evaluate(() => document.querySelector('.command.selected strong')?.textContent);
  if (actual !== expected) throw new Error(`${label}: selected ${actual}, expected ${expected}`);
}
async function reset() {
  await page.goto('http://127.0.0.1:9245/tests/keyboard.html');
  await page.fill(query, 'demo');
  await page.waitForSelector('.command');
  await page.click(query);
}
await reset();
await page.keyboard.press('ArrowDown');
await selected('Beta', 'navigation while provider pending');
await page.evaluate(() => {
  document.querySelector('input').dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true }));
  window.navigationFixture.refresh();
});
await selected('Gamma', 'keyboard event and incremental update in same React batch');
await page.evaluate(() => window.navigationFixture.advance());
await selected('Gamma', 'new result inserted before selection');

await reset();
await page.hover('.command:nth-child(1)');
await page.keyboard.press('ArrowDown');
await selected('Beta', 'keyboard takes over from pointer');
const hovered = await page.evaluate(() => getComputedStyle(document.querySelector('.command:nth-child(1)')).backgroundColor);
if (hovered !== 'rgba(0, 0, 0, 0)') throw new Error('stationary pointer still paints a second highlight');
await page.evaluate(() => window.navigationFixture.advance());
await selected('Beta', 'stationary pointer after list layout changes');
await page.hover('.command:nth-child(4)');
await selected('Gamma', 'real pointer movement still changes selection');
await page.keyboard.press('ArrowUp');
await selected('Beta', 'up arrow while provider pending');
await page.evaluate(() => window.navigationFixture.complete());
await selected('Beta', 'slow provider completion preserves selection');
await page.keyboard.press('Enter');
await page.waitForSelector('h1');
const title = await page.evaluate(() => document.querySelector('h1')?.textContent);
if (title !== 'Beta') throw new Error(`Enter opened ${title}`);
console.log('PASS: pending navigation, same-batch update, stable keys, pointer intent, completion and Enter');
console.log(await page.snapshot());
await task.finish({keep:[]});
