import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { assetName, releaseVersion, targets, verifyAssets } from './release.mjs';

test('发行标签拒绝路径、shell 字符、无效版本和前导零', () => {
  for (const tag of [undefined, 'main', 'v1', 'v01.0.0', 'v1.0.0/../x', 'v1.0.0;id', 'v1.0.0-rc.01']) assert.throws(() => releaseVersion(tag));
  assert.equal(releaseVersion('v1.2.3'), '1.2.3');
  assert.equal(releaseVersion('v1.2.3-rc.1'), '1.2.3-rc.1');
});
test('六目标资产核验拒绝漏包、错包和额外文件', () => {
  const directory = mkdtempSync(join(tmpdir(), 'sinan-release-'));
  try {
    for (const [platform, arch] of targets) {
      const name = assetName('1.2.3', platform, arch);
      const data = Buffer.from(name);
      writeFileSync(join(directory, name), data);
      writeFileSync(join(directory, `${name}.sha256`), `${createHash('sha256').update(data).digest('hex')}  ${name}\n`);
    }
    assert.equal(verifyAssets(directory, '1.2.3').trim().split('\n').length, 6);
    const name = assetName('1.2.3', 'win32', 'arm64');
    writeFileSync(join(directory, name), 'corrupt');
    assert.throws(() => verifyAssets(directory, '1.2.3'), /校验失败/);
    rmSync(join(directory, name));
    assert.throws(() => verifyAssets(directory, '1.2.3'), /不完整/);
    assert.throws(() => assetName('1.2.3', 'linux', 'ia32'));
  } finally { rmSync(directory, { recursive: true, force: true }); }
});
