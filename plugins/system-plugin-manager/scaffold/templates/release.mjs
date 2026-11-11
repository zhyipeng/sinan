// Assemble review metadata from the exact archives, never from mutable latest URLs.
import { readFile, writeFile, readdir } from 'node:fs/promises';
import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';
const manifest = JSON.parse(await readFile('manifest.json', 'utf8'));
const repository = process.env.GITHUB_REPOSITORY;
const tag = process.env.RELEASE_TAG;
const commit = process.env.GITHUB_SHA;
if (!/^[\w.-]+\/[\w.-]+$/.test(repository ?? '') || tag !== `v${manifest.version}` || !/^[a-f0-9]{40}$/.test(commit ?? '')) throw new Error('仓库、标签或提交元数据无效');
const assets = [];
for (const name of (await readdir('dist/release')).filter(name => /^asset-[a-z]+-[a-z0-9]+\.json$/.test(name)).sort()) {
  const asset = JSON.parse(await readFile(`dist/release/${name}`, 'utf8'));
  if (!/^[a-z0-9.-]+\.zip$/.test(asset.file)) throw new Error('非法产物文件名');
  const bytes = await readFile(`dist/release/${asset.file}`);
  if (bytes.length !== asset.size || createHash('sha256').update(bytes).digest('hex') !== asset.sha256) throw new Error('产物摘要不匹配');
  assets.push(asset);
}
if (!assets.length) throw new Error('没有可发布产物');
const metadata = {schemaVersion: 1, pluginId: manifest.id, name: manifest.name, version: manifest.version, repository, tag, commit, compatibility: {apiVersion: manifest.apiVersion, uiBridgeVersion: '2'}, assets};
await writeFile('dist/release/release.json', JSON.stringify(metadata, null, 2) + '\n');
const files = assets.flatMap(asset => [`dist/release/${asset.file}`, `dist/release/${asset.file}.sha256`]);
// Intentionally no --clobber: reruns cannot silently replace a published version.
execFileSync('gh', ['release', 'create', tag, ...files, 'dist/release/release.json', '--verify-tag', '--title', `${manifest.name} ${manifest.version}`, '--generate-notes'], {stdio: 'inherit'});
