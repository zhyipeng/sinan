// Bundle the React business view locally. --dev links it without granting trust.
import { readFile, writeFile, mkdir, copyFile, rm, lstat, readlink, realpath, symlink } from 'node:fs/promises';
import { execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { homedir } from 'node:os';
import path from 'node:path';
import { build } from 'esbuild';
const root = fileURLToPath(new URL('../', import.meta.url));
const out = path.join(root, 'dist/plugin');
let dev = false, pluginDirectory;
const args = process.argv.slice(2);
for (let i = 0; i < args.length; i++) {
  if (args[i] === '--dev') dev = true;
  else if (args[i] === '--plugins-dir' && args[i + 1] && !args[i + 1].startsWith('--')) pluginDirectory = path.resolve(args[++i]);
  else throw new Error('用法：node scripts/build.mjs [--dev [--plugins-dir 目录]]');
}
if (pluginDirectory && !dev) throw new Error('--plugins-dir 需要同时指定 --dev');
const manifest = JSON.parse(await readFile(path.join(root, 'manifest.json'), 'utf8'));
if (!/^[a-z][a-z0-9-]*(\.[a-z][a-z0-9-]*)+$/.test(manifest.id) || manifest.id.length > 128 || manifest.id.startsWith('system.')) throw new Error('插件 ID 无效或使用了保留命名空间');
function defaultPlugins() {
  if (process.platform === 'darwin') return path.join(homedir(), 'Library/Application Support/Sinan/plugins');
  if (process.platform === 'win32') {
    if (!process.env.APPDATA) throw new Error('无法确定 APPDATA，请指定 --plugins-dir');
    return path.join(process.env.APPDATA, 'Sinan/plugins');
  }
  const config = process.env.XDG_CONFIG_HOME || path.join(homedir(), '.config');
  if (!path.isAbsolute(config)) throw new Error('XDG_CONFIG_HOME 必须为绝对路径');
  return path.join(config, 'Sinan/plugins');
}
let link, alreadyLinked = false;
if (dev) {
  pluginDirectory ??= defaultPlugins();
  const relative = path.relative(out, pluginDirectory);
  if (relative === '' || !relative.startsWith(`..${path.sep}`) && relative !== '..' && !path.isAbsolute(relative)) throw new Error('插件安装目录不能位于构建产物内');
  link = path.join(pluginDirectory, manifest.id);
  try {
    const stat = await lstat(link);
    if (!stat.isSymbolicLink()) throw new Error('安装位置已被普通安装占用，请先在宿主中卸载；未覆盖原内容');
    const target = await realpath(link).catch(async () => path.resolve(path.dirname(link), await readlink(link)));
    const expected = await realpath(out).catch(() => out);
    const normalize = value => process.platform === 'win32' ? path.toNamespacedPath(value).toLowerCase() : value;
    if (normalize(target) !== normalize(expected)) throw new Error('安装位置已被其他链接占用，请先在宿主中卸载；未覆盖原内容');
    alreadyLinked = true;
  } catch (error) { if (error.code !== 'ENOENT') throw error; }
}
await rm(out, { recursive: true, force: true });
await mkdir(out, { recursive: true });
await mkdir(path.join(out, 'ui'), { recursive: true });
await build({
  absWorkingDir: root,
  entryPoints: ['frontend/src/main.jsx'],
  outfile: path.join(out, 'ui/app.js'),
  bundle: true,
  format: 'esm',
  platform: 'browser',
  target: ['safari15'],
  jsx: 'automatic',
  // Keep every JS resource below the host's 512 KiB limit, including --dev.
  minify: true,
  sourcemap: false,
  legalComments: 'inline',
  define: { 'process.env.NODE_ENV': JSON.stringify('production') },
});
await copyFile(path.join(root, 'frontend/index.html'), path.join(out, 'ui/index.html'));
if (manifest.runtime.kind === 'process') {
  const { GOOS } = JSON.parse(execFileSync('go', ['env', '-json', 'GOOS'], { cwd: root, encoding: 'utf8' }));
  const currentOS = process.platform === 'win32' ? 'windows' : process.platform;
  if (dev && GOOS !== currentOS) throw new Error('开发链接不能指向其他操作系统的后端，请取消 GOOS 交叉编译设置');
  manifest.runtime.entry = GOOS === 'windows' ? 'backend/plugin.exe' : 'backend/plugin';
  await mkdir(path.join(out, 'backend'), { recursive: true });
  execFileSync('go', ['build', '-o', path.join(out, manifest.runtime.entry), './cmd'], { cwd: root, stdio: 'inherit' });
}
await writeFile(path.join(out, 'manifest.json'), JSON.stringify(manifest, null, 2) + '\n');
console.log(`构建完成：${out}`);
if (dev) {
  await mkdir(pluginDirectory, { recursive: true });
  if (!alreadyLinked) await symlink(out, link, process.platform === 'win32' ? 'junction' : 'dir');
  console.log(`开发链接：${link}`);
  console.log('在插件管理中刷新；Go 插件首次勾选并确认原生信任，重新构建后显式重载。');
}
