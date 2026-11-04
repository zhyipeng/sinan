import { useEffect, useMemo, useState } from 'react';
import { Execute, Install, Refresh, Snapshot, Stop, Uninstall } from '../bindings/sinan/apps/desktop/hostservice';

type Command = { id: string; title: string; keywords?: string[] | null };
type Plugin = { id: string; name: string; version: string; kind: string; state: string; commands: Command[] };
type Data = { plugins: Plugin[]; diagnostics: string[] };
function normalize(snapshot: { plugins: (Omit<Plugin, 'commands'> & { commands: Command[] | null })[] | null; diagnostics: string[] | null }): Data {
  return { plugins: (snapshot.plugins ?? []).map(plugin => ({ ...plugin, commands: plugin.commands ?? [] })), diagnostics: snapshot.diagnostics ?? [] };
}
const states: Record<string, string> = { installed: '已安装', activating: '启动中', activated: '已激活', running: '运行中', deactivated: '已停止', failed: '运行失败' };

export default function App() {
  const [data, setData] = useState<Data>({ plugins: [], diagnostics: [] });
  const [query, setQuery] = useState('');
  const [selected, setSelected] = useState(0);
  const [active, setActive] = useState<{ plugin: Plugin; command: Command }>();
  const [input, setInput] = useState('');
  const [output, setOutput] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [manage, setManage] = useState(false);
  const [directory, setDirectory] = useState('');
  const [trust, setTrust] = useState(false);
  const [connected, setConnected] = useState(false);

  async function reload() { const snapshot = await Refresh(); setData(normalize(snapshot)); setConnected(true); }
  useEffect(() => { Snapshot().then(snapshot => { setData(normalize(snapshot)); setConnected(true); }).catch(() => setError('无法连接桌面宿主，请通过 make run 启动。')); }, []);
  const commands = useMemo(() => data.plugins.flatMap(plugin => plugin.commands.map(command => ({ plugin, command }))).filter(({ plugin, command }) => [plugin.name, command.title, ...(command.keywords ?? [])].join(' ').toLowerCase().includes(query.toLowerCase())), [data, query]);

  async function perform(action: () => Promise<unknown>) {
    setBusy(true); setError('');
    try { await action(); await reload(); } catch (e) { setError(e instanceof Error ? e.message : '操作失败，请重试。'); } finally { setBusy(false); }
  }
  function choose(index: number) { const item = commands[index]; if (item) { setActive(item); setInput(''); setOutput(''); setError(''); } }

  return <main className="shell">
    <header><div className="brand"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="m12 2 7 10-7 10-7-10Z"/><path d="m12 7 3 5-3 5-3-5Z"/></svg><span>司南</span></div><button onClick={() => { setManage(!manage); setActive(undefined); setError(''); }} aria-pressed={manage}>{manage ? '返回命令' : '插件'}</button></header>
    {manage ? <section className="management">
      <div className="section-heading"><h1>本地插件</h1><button disabled={busy || !connected} onClick={() => perform(reload)}>刷新</button></div>
      <div className="plugins">{data.plugins.map(plugin => <div className="plugin" key={plugin.id}><div><strong>{plugin.name}</strong><small>{plugin.version} · {states[plugin.state] ?? plugin.state}</small></div><div className="plugin-actions"><button disabled={busy || !connected || !['running', 'activated', 'failed'].includes(plugin.state)} onClick={() => perform(() => Stop(plugin.id))}>停止</button><button className="danger" disabled={busy || !connected} onClick={() => perform(async () => { await Uninstall(plugin.id); if (active?.plugin.id === plugin.id) setActive(undefined); })}>卸载</button></div></div>)}{!data.plugins.length && <p className="empty">还没有插件，从本地目录安装一个。</p>}</div>
      <form className="install" onSubmit={event => { event.preventDefault(); void perform(async () => { await Install(directory, trust); setDirectory(''); setTrust(false); }); }}>
        <label htmlFor="directory">安装插件</label><div className="install-row"><input id="directory" value={directory} onChange={event => setDirectory(event.target.value)} placeholder="插件目录的绝对路径" required /><button className="primary" disabled={busy || !connected || !directory.trim()}>安装</button></div>
        <label className="trust"><input type="checkbox" checked={trust} onChange={event => setTrust(event.target.checked)} />信任原生插件，允许其以当前用户权限运行</label>
      </form>
      {data.diagnostics.map((diagnostic, index) => <p className="error" key={index}>{diagnostic}</p>)}
    </section> : active ? <section className="tool">
      <button className="back" onClick={() => { setActive(undefined); setOutput(''); setError(''); }}>← 返回</button>
      <h1>{active.command.title}</h1><p className="source">{active.plugin.name}</p>
      <form onSubmit={event => { event.preventDefault(); setOutput(''); void perform(async () => { const result = await Execute(active.plugin.id, active.command.id, input); setOutput(result.text); }); }}>
        {active.plugin.kind === 'process' && <textarea autoFocus aria-label="命令输入" placeholder="输入一段文字…" value={input} onChange={event => setInput(event.target.value)} maxLength={16000} />}
        <button className="primary" disabled={busy || !connected}>{busy ? '执行中…' : '执行'}</button>
      </form>
      {output && <output aria-live="polite">{output}</output>}
    </section> : <section className="launcher">
      <div className="search"><svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="10.5" cy="10.5" r="6.5"/><path d="m16 16 4 4"/></svg><input autoFocus aria-label="查找命令" placeholder="查找命令…" value={query} onChange={event => { setQuery(event.target.value); setSelected(0); }} onKeyDown={event => { if (event.key === 'ArrowDown') { event.preventDefault(); setSelected(value => Math.min(value + 1, commands.length - 1)); } if (event.key === 'ArrowUp') { event.preventDefault(); setSelected(value => Math.max(value - 1, 0)); } if (event.key === 'Enter') choose(selected); if (event.key === 'Escape') { setQuery(''); setSelected(0); } }} /><kbd>↵</kbd></div>
      <div className="command-list" aria-label="命令列表">{commands.map(({ plugin, command }, index) => <button className={`command ${index === selected ? 'selected' : ''}`} key={`${plugin.id}:${command.id}`} onMouseEnter={() => setSelected(index)} onClick={() => choose(index)}><span className="command-icon">{plugin.kind === 'process' ? '↔' : '✧'}</span><span><strong>{command.title}</strong><small>{plugin.name}</small></span><span className="open">↵</span></button>)}{!commands.length && <div className="empty">{data.plugins.length ? '没有匹配的命令' : '安装插件，开始使用司南。'}</div>}</div>
    </section>}
    {error && <p className="error" role="alert">{error}</p>}
    <footer><span className={`status ${connected ? 'online' : ''}`} />{connected ? `${data.plugins.length} 个插件` : '宿主未连接'}<span className="footer-hint">{manage ? '本地安装' : '↑ ↓ 选择　↵ 打开'}</span></footer>
  </main>;
}
