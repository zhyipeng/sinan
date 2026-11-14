#!/usr/bin/env node
import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { existsSync, mkdirSync, readFileSync, readdirSync, writeFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const output = join(root, 'dist/release');
export const targets = [['darwin', 'arm64'], ['darwin', 'x64'], ['win32', 'x64'], ['win32', 'arm64'], ['linux', 'x64'], ['linux', 'arm64']];
export function releaseVersion(tag) {
  if (!/^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-(?:[A-Za-z0-9-]+\.)*[A-Za-z0-9-]+)?$/.test(tag ?? '')) throw new Error('标签格式应为 v<major>.<minor>.<patch>[-prerelease]');
  const version = tag.slice(1);
  if (version.includes('-') && version.split('-').slice(1).join('-').split('.').some(part => /^\d+$/.test(part) && part.length > 1 && part.startsWith('0'))) throw new Error('预发布数字标识不能有前导零');
  return version;
}
export function assetName(version, platform, arch) {
  if (!targets.some(target => target[0] === platform && target[1] === arch)) throw new Error('不支持的发行目标');
  const os = { darwin: 'macos', win32: 'windows', linux: 'linux' }[platform];
  return `Sinan-${version}-${os}-${arch === 'x64' ? 'amd64' : arch}.${platform === 'win32' ? 'zip' : 'tar.gz'}`;
}
function command(name, args, options = {}) {
  const result = spawnSync(name, args, { cwd: root, stdio: 'inherit', ...options });
  if (result.error || result.status !== 0) throw new Error(`${name} 执行失败`);
  return result;
}
const digest = data => createHash('sha256').update(data).digest('hex');
export function verifyAssets(directory, version) {
  const expected = targets.map(([platform, arch]) => assetName(version, platform, arch));
  const files = readdirSync(directory).sort();
  const allowed = expected.flatMap(name => [name, `${name}.sha256`]).sort();
  if (JSON.stringify(files) !== JSON.stringify(allowed)) throw new Error('发行资产不完整或包含额外文件');
  return expected.map(name => {
    const hash = digest(readFileSync(join(directory, name)));
    if (readFileSync(join(directory, `${name}.sha256`), 'utf8') !== `${hash}  ${name}\n`) throw new Error(`校验失败：${name}`);
    return `${hash}  ${name}\n`;
  }).join('');
}
function archive(version) {
  const name = assetName(version, process.platform, process.arch);
  mkdirSync(output, { recursive: true });
  const source = process.platform === 'darwin' ? 'Sinan.app' : `Sinan-${process.platform}-${process.arch}`;
  const directory = join(root, 'bin', source);
  if (!existsSync(directory)) throw new Error('请先执行 node scripts/desktop.mjs package');
  const metadataDir = process.platform === 'darwin' ? join(directory, 'Contents/Resources') : directory;
  const metadata = JSON.parse(readFileSync(join(metadataDir, 'release.json'), 'utf8'));
  if (metadata.version !== version || metadata.platform !== process.platform || metadata.arch !== process.arch) throw new Error('打包元数据与当前发行目标不符，请使用同一 RELEASE_TAG 重新打包');
  if (process.platform === 'darwin') {
    // Ad-hoc only: no Developer ID certificate, notarization or secret required.
    command('codesign', ['--force', '--deep', '--sign', '-', directory]);
    command('codesign', ['--verify', '--deep', '--strict', directory]);
  }
  if (process.platform === 'win32') {
    command('powershell.exe', ['-NoProfile', '-NonInteractive', '-Command', 'Compress-Archive -LiteralPath $env:SINAN_ARCHIVE_SOURCE -DestinationPath $env:SINAN_ARCHIVE_DEST -Force'], {
      env: { ...process.env, SINAN_ARCHIVE_SOURCE: directory, SINAN_ARCHIVE_DEST: join(output, name) },
    });
  } else {
    command('tar', ['-czf', join(output, name), '-C', join(root, 'bin'), source], {
      env: process.platform === 'darwin' ? { ...process.env, COPYFILE_DISABLE: '1' } : process.env,
    });
  }
  writeFileSync(join(output, `${name}.sha256`), `${digest(readFileSync(join(output, name)))}  ${name}\n`);
}
function publish(tag, version) {
  const checksums = verifyAssets(output, version);
  const existing = spawnSync('gh', ['release', 'view', tag, '--json', 'isDraft'], { cwd: root, encoding: 'utf8' });
  if (existing.status === 0 && !JSON.parse(existing.stdout).isDraft) throw new Error('该版本已公开发布，拒绝覆盖；请创建新版本标签');
  if (existing.status !== 0) {
    // Confirm access and tag existence separately; never create a missing tag.
    command('gh', ['release', 'create', tag, '--draft', '--verify-tag', '--title', `Sinan ${tag}`, '--generate-notes', '--notes', releaseNotes]);
  }
  writeFileSync(join(output, 'SHA256SUMS.txt'), checksums);
  const files = [...targets.map(([platform, arch]) => assetName(version, platform, arch)), 'SHA256SUMS.txt'];
  // Only drafts can be resumed; upload failures leave an unpublished release.
  command('gh', ['release', 'upload', tag, ...files.map(name => join(output, name)), '--clobber']);
  const assets = command('gh', ['release', 'view', tag, '--json', 'assets'], { encoding: 'utf8', stdio: 'pipe' });
  const actual = JSON.parse(assets.stdout).assets;
  if (actual.length !== files.length || files.some(name => !actual.some(asset => asset.name === name && asset.size === readFileSync(join(output, name)).length))) throw new Error('远端草稿资产不完整，请检查后重试');
  command('gh', ['release', 'edit', tag, '--draft=false', ...(version.includes('-') ? ['--prerelease', '--latest=false'] : ['--prerelease=false'])]);
}
const releaseNotes = `三平台便携构建，内嵌应用检索、设置和插件中心；首次正常启动安装缺失的内置插件，已有安装保留。退出旧实例后再替换应用，用户配置与数据不在发行目录内。

macOS：解压 Sinan.app 后移入 Applications。仅 ad-hoc 签名，未经 Developer ID 签名或公证，Gatekeeper 可能阻止打开。
Windows：解压整个目录，运行 sinan.exe；需要 WebView2 Runtime 119+。无安装器或 Authenticode 签名。
Linux：Ubuntu 24.04 基线，需要 GTK3、WebKitGTK 4.1（引擎 2.40+）和 GIO，解压后运行 sinan。Wayland 的跨应用自动输入不可用。

各目标的构建和无界面检查不等于真实桌面、输入法、焦点、多屏或安装升级验收；详见仓库 docs/cross-platform.md。下载后请使用 SHA256SUMS.txt 校验。`;

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  try {
    const tag = process.env.RELEASE_TAG;
    const version = releaseVersion(tag);
    switch (process.argv[2]) {
      case 'validate':
        if (process.argv[3] !== process.platform || process.argv[4] !== process.arch) throw new Error('runner 与目标平台/架构不符，拒绝误标发行资产');
        assetName(version, process.platform, process.arch);
        break;
      case 'archive': archive(version); break;
      case 'publish': publish(tag, version); break;
      default: throw new Error('用法：node scripts/release.mjs validate <platform> <arch>|archive|publish');
    }
  } catch (error) { console.error(error.message); process.exitCode = 1; }
}
