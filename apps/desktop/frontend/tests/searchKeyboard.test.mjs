import { test } from 'node:test';
import assert from 'node:assert/strict';
import { isSearchOpenKey } from '../src/searchKeyboard.ts';

const key = (value, overrides = {}) => ({ key: value, keyCode: 0, isComposing: false, repeat: false, ctrlKey: false, metaKey: false, altKey: false, shiftKey: false, ...overrides });

test('关闭时空格继续输入，开启时空格与回车共用打开路径', () => {
  assert.equal(isSearchOpenKey(key(' '), false), false);
  assert.equal(isSearchOpenKey(key(' '), true), true);
  for (const enabled of [false, true]) {
    assert.equal(isSearchOpenKey(key('Enter'), enabled), true);
    assert.equal(isSearchOpenKey(key('a'), enabled), false);
  }
});

test('输入法组合输入和 WebKit 229 事件不打开选中项', () => {
  for (const value of [' ', 'Enter']) {
    assert.equal(isSearchOpenKey(key(value, {isComposing: true}), true), false);
    assert.equal(isSearchOpenKey(key(value, {keyCode: 229}), true), false);
  }
});

test('空格组合键和长按重复事件保留原行为', () => {
  for (const property of ['ctrlKey', 'metaKey', 'altKey', 'shiftKey', 'repeat']) {
    assert.equal(isSearchOpenKey(key(' ', {[property]: true}), true), false);
  }
});
