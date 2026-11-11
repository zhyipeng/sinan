// Optional cross-repository PR. GH_TOKEN must be scoped to the registry repository.
import { readFile } from 'node:fs/promises';
import { execFileSync } from 'node:child_process';
const registry = process.env.SINAN_REGISTRY_REPOSITORY;
if (!/^[\w.-]+\/[\w.-]+$/.test(registry ?? '')) throw new Error('请配置 SINAN_REGISTRY_REPOSITORY=owner/repo');
const submission = JSON.parse(await readFile('dist/release/release.json', 'utf8'));
if (!/^[a-z][a-z0-9-]*(\.[a-z][a-z0-9-]*)+$/.test(submission.pluginId) || !/^\d+\.\d+\.\d+$/.test(submission.version)) throw new Error('发布资料无效');
function api(endpoint, method = 'GET', body) {
  return JSON.parse(execFileSync('gh', ['api', endpoint, '--method', method, ...(body ? ['--input', '-'] : [])], {input: body ? JSON.stringify(body) : undefined, encoding: 'utf8', stdio: ['pipe', 'pipe', 'pipe']}));
}
function optional(endpoint) {
  try { return api(endpoint); }
  catch (error) { if (/HTTP 404/.test(String(error.stderr))) return null; throw new Error('GitHub API 请求失败，请检查仓库配置和授权范围'); }
}
const released = api(`repos/${submission.repository}/releases/tags/${encodeURIComponent(submission.tag)}`);
submission.releaseId = released.id;
for (const asset of submission.assets) {
  const remote = released.assets.find(item => item.name === asset.file);
  if (!remote || remote.size !== asset.size || remote.digest && remote.digest !== `sha256:${asset.sha256}`) throw new Error('GitHub Release 资产与构建记录不匹配');
  asset.assetId = remote.id;
  asset.downloadUrl = remote.browser_download_url;
}
const repo = api(`repos/${registry}`);
const base = repo.default_branch;
const branch = `plugin-updates/${submission.pluginId}/${submission.version}`;
if (!optional(`repos/${registry}/git/ref/heads/${branch}`)) {
  const source = api(`repos/${registry}/git/ref/heads/${base}`);
  api(`repos/${registry}/git/refs`, 'POST', {ref: `refs/heads/${branch}`, sha: source.object.sha});
}
const file = `submissions/${submission.pluginId}/${submission.version}.json`;
const existing = optional(`repos/${registry}/contents/${file}?ref=${encodeURIComponent(branch)}`);
const content = Buffer.from(JSON.stringify(submission, null, 2) + '\n').toString('base64');
if (existing && existing.content.replace(/\s/g, '') !== content) throw new Error('已有同版本提交内容不同，请发布新版本，未覆盖旧记录');
if (!existing) api(`repos/${registry}/contents/${file}`, 'PUT', {message: `Register ${submission.pluginId} ${submission.version}`, content, branch});
const head = `${registry.split('/')[0]}:${branch}`;
const prs = api(`repos/${registry}/pulls?state=open&head=${encodeURIComponent(head)}&base=${encodeURIComponent(base)}`);
if (!prs.length) api(`repos/${registry}/pulls`, 'POST', {title: `${submission.pluginId} ${submission.version}`, head: branch, base, body: '提交已发布版本的固定 Release/Asset 标识、平台产物与摘要，供目录维护者审核。此记录不直接进入可安装索引，不声明官方身份。'});
console.log('目录发布审核 PR 已就绪');
