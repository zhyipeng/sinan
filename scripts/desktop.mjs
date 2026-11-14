#!/usr/bin/env node
// Native, shell-free entry point shared by Make, PowerShell and CI.
import { spawnSync } from 'node:child_process';
import { cpSync, existsSync, mkdirSync, readFileSync, renameSync, rmSync, writeFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { releaseVersion } from './release.mjs';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const platform = process.platform;
const suffix = platform === 'win32' ? '.exe' : '';
const env = { ...process.env };
if (platform === 'darwin') {
  for (const key of ['CGO_CFLAGS', 'CGO_LDFLAGS']) {
    env[key] = `${env[key] || ''} -mmacosx-version-min=12.0`.trim();
  }
  env.MACOSX_DEPLOYMENT_TARGET = '12.0';
}
// The pinned Wails version supports GTK3/WebKit 4.1 on Ubuntu 24.04.
const tags = platform === 'linux' ? ['-tags', 'gtk3'] : [];
function run(command, args = [], extraEnv = {}) {
  const result = spawnSync(command, args, { cwd: root, env: { ...env, ...extraEnv }, stdio: 'inherit' });
  if (result.error) throw result.error;
  if (result.status !== 0) throw new Error(`${command} 执行失败（${result.status ?? result.signal}）`);
}
// npm's JS entry avoids passing user paths through cmd.exe on Windows.
function npm(args) {
  if (platform !== 'win32') return run('npm', args);
  const result = spawnSync('where.exe', ['npm.cmd'], { encoding: 'utf8', env });
  if (result.status !== 0) throw new Error('找不到 npm，请安装 Node.js');
  const paths = result.stdout.trim().split(/\r?\n/);
  const cli = paths.map(path => join(dirname(path), 'node_modules/npm/bin/npm-cli.js')).find(existsSync);
  if (!cli) throw new Error('找不到 npm-cli.js，请检查 Node.js 安装');
  run(process.execPath, [cli, ...args]);
}
const binary = name => join(root, 'bin', name + suffix);
function bindings() {
  mkdirSync(join(root, 'apps/desktop/frontend/dist'), { recursive: true });
  writeFileSync(join(root, 'apps/desktop/frontend/dist/.placeholder'), '');
  run('wails3', ['generate', 'bindings', ...(tags.length ? ['-f', tags.join(' ')] : []), '-ts', '-i', '-noevents', '-d', 'apps/desktop/frontend/bindings', './apps/desktop']);
}
function frontend() {
  bindings();
  npm(['--prefix', 'apps/desktop/frontend', 'run', 'build']);
}
function build() {
  frontend();
  bundlePlugins();
  mkdirSync(join(root, 'bin'), { recursive: true });
  const version = process.env.RELEASE_TAG ? releaseVersion(process.env.RELEASE_TAG) : 'dev';
  for (const [name, source] of [['sinan', 'desktop'], ['sinanctl', 'cli'], ['sinan-ui-host', 'ui-host'], ['sinan-updater', 'updater']]) {
    const ldflags = [`-X=sinan/internal/buildinfo.Version=${version}`];
    if (platform === 'win32' && (name === 'sinan' || name === 'sinan-updater')) ldflags.push('-H=windowsgui');
    run('go', ['build', ...tags, '-ldflags', ldflags.join(' '), '-o', binary(name), `./apps/${source}`]);
  }
}
function bundlePlugins() {
  const destination = join(root, 'apps/desktop/bundled');
  for (const name of ['system-app-search', 'system-settings', 'system-plugin-manager']) {
    const source = join(root, 'plugins', name);
    const manifest = JSON.parse(readFileSync(join(source, 'manifest.json'), 'utf8'));
    const target = join(destination, manifest.id);
    rmSync(target, { recursive: true, force: true });
    mkdirSync(target, { recursive: true });
    if (name === 'system-app-search') {
      manifest.runtime.entry = `backend/plugin${suffix}`;
      mkdirSync(join(target, 'backend'), { recursive: true });
      run('go', ['build', '-o', join(target, manifest.runtime.entry), './plugins/system-app-search/cmd']);
    }
    writeFileSync(join(target, 'manifest.json'), JSON.stringify(manifest, null, 2) + '\n');
    cpSync(join(source, 'icon.svg'), join(target, 'icon.svg'));
  }
}
function prepare() {
  const dir = join(root, '.sinan-dev/packages/app-search');
  mkdirSync(join(dir, 'backend'), { recursive: true });
  const manifest = JSON.parse(readFileSync(join(root, 'plugins/system-app-search/manifest.json'), 'utf8'));
  manifest.runtime.entry = `backend/plugin${suffix}`;
  writeFileSync(join(dir, 'manifest.json'), JSON.stringify(manifest, null, 2) + '\n');
  cpSync(join(root, 'plugins/system-app-search/icon.svg'), join(dir, 'icon.svg'));
  run('go', ['build', '-o', join(dir, 'backend', 'plugin' + suffix), './plugins/system-app-search/cmd']);
  for (const [id, source, native] of [
    ['system.app-search', dir, true],
    ['system.settings', 'plugins/system-settings', false],
    ['system.plugin-manager', 'plugins/system-plugin-manager', false],
  ]) {
    const operation = existsSync(join(root, '.sinan-dev/plugins', id)) ? 'upgrade' : 'install';
    run(binary('sinanctl'), ['-plugins', '.sinan-dev/plugins', ...(native ? ['-trust-native'] : []), operation, source]);
  }
}
function packageDesktop() {
  // Validate before spending time building or touching an existing package.
  const version = process.env.RELEASE_TAG ? releaseVersion(process.env.RELEASE_TAG) : undefined;
  build();
  if (platform === 'darwin') {
    const contents = join(root, 'bin/Sinan.app/Contents');
    rmSync(dirname(contents), { recursive: true, force: true });
    mkdirSync(join(contents, 'MacOS'), { recursive: true });
    mkdirSync(join(contents, 'Resources'), { recursive: true });
    for (const name of ['sinan', 'sinanctl', 'sinan-ui-host', 'sinan-updater']) {
      const destination = join(contents, 'MacOS', name);
      cpSync(binary(name), destination + '.next');
      renameSync(destination + '.next', destination);
    }
    let plist = readFileSync(join(root, 'apps/desktop/Info.plist'), 'utf8');
    if (version) {
      plist = plist.replace(/(<key>CFBundleShortVersionString<\/key><string>)[^<]+/, (_, prefix) => prefix + version.split('-')[0])
        .replace(/(<key>CFBundleVersion<\/key><string>)[^<]+/, (_, prefix) => prefix + version.split('-')[0]);
    }
    writeFileSync(join(contents, 'Info.plist'), plist);
    cpSync(join(root, 'apps/desktop/assets/AppIcon.icns'), join(contents, 'Resources/AppIcon.icns'));
  } else {
    // Portable build directory only; OS installers and dependency bundling are pending.
    const destination = join(root, 'bin', `Sinan-${platform}-${process.arch}`);
    rmSync(destination, { recursive: true, force: true });
    mkdirSync(destination, { recursive: true });
    for (const name of ['sinan', 'sinanctl', 'sinan-ui-host', 'sinan-updater']) cpSync(binary(name), join(destination, name + suffix));
  }
  const metadataDir = platform === 'darwin' ? join(root, 'bin/Sinan.app/Contents/Resources') : join(root, 'bin', `Sinan-${platform}-${process.arch}`);
  writeFileSync(join(metadataDir, 'release.json'), JSON.stringify({
    version: version ?? JSON.parse(readFileSync(join(root, 'apps/desktop/frontend/package.json'), 'utf8')).version,
    platform,
    arch: process.arch,
  }, null, 2) + '\n');
}

const tasks = {
  setup() { run('go', ['mod', 'download']); npm(['--prefix', 'apps/desktop/frontend', 'ci']); },
  bindings,
  frontend,
  build,
  prepare,
  package: packageDesktop,
  run() { build(); run(binary('sinan')); },
  dev() { build(); prepare(); run(binary('sinan'), ['-plugins', '.sinan-dev/plugins']); },
  test() {
    frontend();
    run(process.execPath, ['--test', 'packages/plugin-sdk/theme/runtime.test.mjs']);
    run(process.execPath, ['--test', 'internal/plugin/uihost/gtk_bridge.test.mjs', 'internal/plugin/uihost/webview_bridge.test.mjs']);
    run(process.execPath, ['--experimental-strip-types', '--test', 'apps/desktop/frontend/tests/paste.test.mjs']);
    run('go', ['test', ...tags, '-race', '-p', '1', './...'], { SINAN_NATIVE_UI_TEST: '0' });
  },
};
try {
  const task = tasks[process.argv[2]];
  if (!Object.hasOwn(tasks, process.argv[2])) throw new Error(`用法：node scripts/desktop.mjs ${Object.keys(tasks).join('|')}`);
  task();
} catch (error) {
  console.error(error.message);
  process.exitCode = 1;
}
