import { useEffect, useMemo, useRef, useState } from 'react';
import { Events } from '@wailsio/runtime';
import { Execute, Install, LauncherStatus, Refresh, ResizeLauncher, Snapshot, Stop, Uninstall } from '../bindings/sinan/apps/desktop/hostservice';

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
  const [shortcutError, setShortcutError] = useState('');
  const shellRef = useRef<HTMLElement>(null);
  const searchRef = useRef<HTMLInputElement>(null);
  const commandRefs = useRef<(HTMLButtonElement | null)[]>([]);
  // Invalidates late UI results when a running command is dismissed and reopened.
  const generation = useRef(0);

  async function reload() {
    const snapshot = await Refresh();
    setData(normalize(snapshot));
    setConnected(true);
  }

  useEffect(() => {
    Snapshot().then(snapshot => { setData(normalize(snapshot)); setConnected(true); })
      .catch(() => setError('无法连接桌面宿主，请重新启动司南。'));
    LauncherStatus().then(setShortcutError).catch(() => {});
    const reset = () => {
      generation.current++;
      setManage(false); setActive(undefined); setQuery(''); setSelected(0);
      setInput(''); setOutput(''); setError(''); setBusy(false);
      void LauncherStatus().then(setShortcutError).catch(() => {});
      void reload().catch(() => setError('无法读取插件，请重试。'));
      requestAnimationFrame(() => searchRef.current?.focus());
    };
    return Events.On('launcher:show', reset);
  }, []);

  useEffect(() => {
    const shell = shellRef.current;
    if (!shell) return;
    let previous = 0;
    const observer = new ResizeObserver(() => {
      const height = Math.ceil(shell.getBoundingClientRect().height);
      if (height === previous) return;
      previous = height;
      void ResizeLauncher(height).catch(() => {});
    });
    observer.observe(shell);
    return () => observer.disconnect();
  }, []);

  const hasQuery = query.trim().length > 0;
  const commands = useMemo(() => data.plugins.flatMap(plugin => plugin.commands.map(command => ({ plugin, command })))
    .filter(({ plugin, command }) => [plugin.name, command.title, ...(command.keywords ?? [])].join(' ').toLowerCase().includes(query.trim().toLowerCase())), [data, query]);

  useEffect(() => { setSelected(value => Math.max(0, Math.min(value, commands.length - 1))); }, [commands.length]);
  useEffect(() => { commandRefs.current[selected]?.scrollIntoView({ block: 'nearest' }); }, [selected]);

  async function perform(action: () => Promise<unknown>) {
    const current = generation.current;
    setBusy(true); setError('');
    try {
      await action();
      await reload();
    } catch (e) {
      if (current === generation.current) setError(e instanceof Error ? e.message : '操作失败，请重试。');
    } finally {
      if (current === generation.current) setBusy(false);
    }
  }

  function choose(index: number) {
    const item = commands[index];
    if (item) { setActive(item); setInput(''); setOutput(''); setError(''); }
  }

  return <main ref={shellRef} className="shell">
    {manage ? <section className="management">
      <div className="view-header"><button className="back" onClick={() => setManage(false)} aria-label="返回搜索">←</button><h1>本地插件</h1><button disabled={busy || !connected} onClick={() => perform(reload)}>刷新</button></div>
      <div className="management-body">
        <div className="plugins">
          {data.plugins.map(plugin => <div className="plugin" key={plugin.id}>
            <div><strong>{plugin.name}</strong><small>{plugin.version} · {states[plugin.state] ?? plugin.state}</small></div>
            <div className="plugin-actions"><button disabled={busy || !connected || !['running', 'activated', 'failed'].includes(plugin.state)} onClick={() => perform(() => Stop(plugin.id))}>停止</button><button className="danger" disabled={busy || !connected} onClick={() => perform(async () => { await Uninstall(plugin.id); if (active?.plugin.id === plugin.id) setActive(undefined); })}>卸载</button></div>
          </div>)}
          {!data.plugins.length && <p className="empty">还没有插件，从本地目录安装一个。</p>}
        </div>
        <form className="install" onSubmit={event => { event.preventDefault(); void perform(async () => { await Install(directory, trust); setDirectory(''); setTrust(false); }); }}>
          <label htmlFor="directory">安装插件</label>
          <div className="install-row"><input id="directory" value={directory} onChange={event => setDirectory(event.target.value)} placeholder="插件目录的绝对路径" required /><button className="primary" disabled={busy || !connected || !directory.trim()}>安装</button></div>
          <label className="trust"><input type="checkbox" checked={trust} onChange={event => setTrust(event.target.checked)} />信任原生插件，允许其以当前用户权限运行</label>
        </form>
        {data.diagnostics.map((diagnostic, index) => <p className="error" key={index}>{diagnostic}</p>)}
        {shortcutError && <p className="error">{shortcutError}</p>}
      </div>
    </section> : active ? <section className="tool">
      <div className="view-header"><button className="back" aria-label="返回搜索" onClick={() => { generation.current++; setBusy(false); setActive(undefined); setOutput(''); setError(''); }}>←</button><h1>{active.command.title}</h1><span className="source">{active.plugin.name}</span></div>
      <div className="tool-body">
        <form onSubmit={event => {
          event.preventDefault();
          const current = generation.current;
          setOutput('');
          void perform(async () => {
            const result = await Execute(active.plugin.id, active.command.id, input);
            if (current === generation.current) setOutput(result.text);
          });
        }}>
          {active.plugin.kind === 'process' && <textarea autoFocus aria-label="命令输入" placeholder="输入一段文字…" value={input} onChange={event => setInput(event.target.value)} maxLength={16000} />}
          <button className="primary" disabled={busy || !connected}>{busy ? '执行中…' : '执行'}</button>
        </form>
        {output && <output aria-live="polite">{output}</output>}
      </div>
    </section> : <section className="launcher">
      <div className="search">
        <svg className="compass" viewBox="0 0 24 24" aria-hidden="true"><path d="m12 2 7 10-7 10-7-10Z"/><path d="m12 7 3 5-3 5-3-5Z"/></svg>
        <input ref={searchRef} autoFocus aria-label="搜索命令" placeholder="搜索…" value={query} onChange={event => { setQuery(event.target.value); setSelected(0); }} onKeyDown={event => {
          if (event.nativeEvent.isComposing) return;
          if (event.key === 'ArrowDown') { event.preventDefault(); setSelected(value => Math.min(value + 1, Math.max(0, commands.length - 1))); }
          if (event.key === 'ArrowUp') { event.preventDefault(); setSelected(value => Math.max(value - 1, 0)); }
          if (event.key === 'Enter' && hasQuery) { event.preventDefault(); choose(selected); }
        }} />
        <button className="manage-button" aria-label="管理插件" title="管理插件" onClick={() => { setManage(true); setError(''); }}>···{shortcutError && <span className="warning-dot" />}</button>
      </div>
      {hasQuery && <>
        <div className="command-list" aria-label="搜索结果">
          {commands.map(({ plugin, command }, index) => <button ref={element => { commandRefs.current[index] = element; }} className={`command ${index === selected ? 'selected' : ''}`} key={`${plugin.id}:${command.id}`} onMouseEnter={() => setSelected(index)} onClick={() => choose(index)}>
            <span className="command-icon">{plugin.kind === 'process' ? '↔' : '✧'}</span><span><strong>{command.title}</strong><small>{plugin.name}</small></span><span className="open">↵</span>
          </button>)}
          {!commands.length && <div className="empty">没有匹配的命令</div>}
        </div>
        <footer><span>{commands.length} 个结果</span><span>↑ ↓ 选择　↵ 打开　esc 隐藏</span></footer>
      </>}
    </section>}
    {error && <p className="error" role="alert">{error}</p>}
  </main>;
}
