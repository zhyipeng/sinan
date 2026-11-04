// Run against `npm run dev`; requires the local ego-browser CLI.
const task = await taskSpace('Sinan default selection regression');
const page = task.page('p1');
const query = 'input[aria-label="搜索命令"]';
async function selected(expected, label) {
  await page.waitForFunction(title => document.querySelector('.command.selected strong')?.textContent === title, expected, {timeout:2000});
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
await selected('Alpha', 'first batch defaults to first result');
await page.evaluate(() => window.navigationFixture.advance());
await selected('Late', 'untouched selection follows new top-ranked result');
await page.keyboard.press('Enter');
await page.waitForSelector('h1');
if (await page.evaluate(() => document.querySelector('h1')?.textContent) !== 'Late') throw new Error('Enter opened an earlier result instead of the new top result');

await reset();
await page.keyboard.press('ArrowUp');
await page.evaluate(() => window.navigationFixture.advance());
await selected('Alpha', 'manual choice at first-row boundary remains pinned');
await page.fill(query, 'another query');
await page.waitForFunction(() => document.querySelectorAll('.command').length === 3);
await page.evaluate(() => window.navigationFixture.advance());
await selected('Late', 'new query restores automatic selection');

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
await page.goto('http://127.0.0.1:9245/tests/keyboard.html');
await page.fill(query, 'apps');
await page.waitForSelector('.command');
await page.waitForFunction(() => document.querySelector('.command-icon img')?.naturalWidth === 64);
await page.click(query);
await page.keyboard.press('Enter');
await page.waitForFunction(() => window.navigationFixture.counts().dismissals === 1);
const direct = await page.evaluate(() => ({...window.navigationFixture.counts(), form: !!document.querySelector('textarea')}));
if (direct.executions !== 1 || direct.form) throw new Error(`Direct execution failed: ${JSON.stringify(direct)}`);
console.log('PASS: direct action executes once and dismisses without opening a command form');
console.log('PASS: pending navigation, same-batch update, stable keys, pointer intent, completion and Enter');
console.log(await page.snapshot());
await task.finish({keep:[]});
