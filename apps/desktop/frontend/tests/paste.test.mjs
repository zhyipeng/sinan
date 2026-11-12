import { test } from 'node:test';
import assert from 'node:assert/strict';
import { pasteBackspaceText, filterPasteResults, escapeAction, disposePaste } from '../src/paste.ts';

test('退格退出：30 个 Unicode 字符以内恢复原文，长文本和图片清空', () => {
  for (const text of ['short', '中'.repeat(30), '😀'.repeat(30), 'a\nb']) {
    assert.equal(pasteBackspaceText({kind:'text',text}),text);
  }
  assert.equal(pasteBackspaceText({kind:'text',text:'中'.repeat(31)}),'');
  assert.equal(pasteBackspaceText({kind:'image',dataBase64:'',previewURL:'blob:fixture',width:1,height:1}),'');
});
test('筛选仅缩小已匹配结果，支持标题、来源与关键词', () => {
  const items=[{title:'JSON 工具',subtitle:'官方插件',pluginId:'tools.json',commandId:'open'},{title:'图片工具',subtitle:'图像',pluginId:'tools.image',commandId:'open'}];
  const plugins=[{id:'tools.json',name:'JSON 工具',commands:[{id:'open',keywords:['格式化','压缩']}]},{id:'tools.image',name:'图片工具',commands:[{id:'open'}]}];
  assert.deepEqual(filterPasteResults(items,'格式化',plugins),[items[0]]);
  assert.deepEqual(filterPasteResults(items,'json 官方',plugins),[items[0]]);
  assert.deepEqual(filterPasteResults(items,'图片',plugins),[items[1]]);
  assert.deepEqual(filterPasteResults(items,'不存在',plugins),[]);
  assert.deepEqual(filterPasteResults(items,'',plugins),items);
  assert.deepEqual(filterPasteResults([],'JSON',plugins),[]);
});
test('Esc 先退出模式并清空，空主输入框再次 Esc 才隐藏', () => {
  assert.equal(escapeAction(true,''),'reset');
  assert.equal(escapeAction(true,'filter'),'reset');
  assert.equal(escapeAction(false,'text'),'reset');
  assert.equal(escapeAction(false,'   '),'reset');
  assert.equal(escapeAction(false,''),'hide');
});
test('退出图片粘贴释放预览地址，普通文本不触碰图片资源', () => {
  const original=URL.revokeObjectURL,urls=[];
  URL.revokeObjectURL=url=>urls.push(url);
  try {
    disposePaste({kind:'image',previewURL:'blob:fixture',dataBase64:'',width:1,height:1});
    disposePaste({kind:'text',text:'x'});disposePaste();
    assert.deepEqual(urls,['blob:fixture']);
  } finally {URL.revokeObjectURL=original;}
});
