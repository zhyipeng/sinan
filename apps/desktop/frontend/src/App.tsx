import { useEffect, useRef, useState } from 'react';
import { Events } from '@wailsio/runtime';
import { pluginViews, settingsBridge } from './pluginViews';
import type { SettingsSchema } from '../../../../plugins/system-settings/frontend/Settings';
import { Execute, PreviewPackage, ApplyPackage, Reload, LauncherStatus, Refresh, ResizeLauncher, Snapshot, Stop, Uninstall, StartSearch, CancelSearch, HideLauncher } from '../bindings/sinan/apps/desktop/hostservice';

type Command = { view?: string; id: string; title: string; keywords?: string[] | null; execution?: string };
type PackagePreview = { id: string; name: string; version: string; kind: string; installedVersion: string; fingerprint: string; installedFingerprint: string; changed: boolean };
type Plugin = { changed?: boolean; id: string; name: string; version: string; kind: string; state: string; settings?: SettingsSchema[] | null; commands: Command[] };
type SearchItem = { icon?: string; key: string; pluginId: string; providerId: string; commandId: string; input: string; title: string; subtitle: string; kind: string; score: number };
type SearchUpdate = { queryId: string; items: SearchItem[]; pending: number; failures: string[]; done: boolean };
type Data = { plugins: Plugin[]; diagnostics: string[] };
function normalize(snapshot: { plugins: (Omit<Plugin, 'commands'> & { commands: Command[] | null })[] | null; diagnostics: string[] | null }): Data {
  return { plugins: (snapshot.plugins ?? []).map(plugin => ({ ...plugin, commands: plugin.commands ?? [] })), diagnostics: snapshot.diagnostics ?? [] };
}
const states: Record<string, string> = { installed: '已安装', activating: '启动中', activated: '已激活', running: '运行中', deactivated: '已停止', failed: '运行失败' };

export default function App() {
  const [data, setData] = useState<Data>({ plugins: [], diagnostics: [] });
  const [query, setQuery] = useState('');
  const [commands, setCommands] = useState<SearchItem[]>([]);
  const [searching, setSearching] = useState(false);
  const [searchFailures, setSearchFailures] = useState(0);
  const searchSequence = useRef(Date.now() * 1000);
  const activeSequence = useRef('');
  const [selection, setSelection] = useState({ key: '', manual: false });
  const pointerPosition = useRef<{ x: number; y: number }>();
  const [active, setActive] = useState<{ plugin: Plugin; command: Command }>();
  const [input, setInput] = useState('');
  const [output, setOutput] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [manage, setManage] = useState(false);
  const [directory, setDirectory] = useState('');
  const [trust, setTrust] = useState(false);
  const [preview, setPreview] = useState<PackagePreview>();
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
      setManage(false); setActive(undefined); setQuery(''); setSelection({ key: '', manual: false });
      setInput(''); setOutput(''); setError(''); setBusy(false);
      setDirectory(''); setTrust(false); setPreview(undefined);
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
  useEffect(() => Events.On('search:update', event => {
    const update = event.data as SearchUpdate;
    if (update.queryId !== activeSequence.current) return;
    const next = update.items ?? [];
    setCommands(next);
    // Follow ranking until the user chooses a row. Keep key and intent in one
    // state update so keyboard events and provider results can safely share a batch.
    setSelection(current => current.manual && next.some(item => item.key === current.key)
      ? current : { key: next[0]?.key ?? '', manual: false });
    setSearching(!update.done);
    setSearchFailures(update.failures?.length ?? 0);
  }), []);

  useEffect(() => {
    const sequence = ++searchSequence.current;
    activeSequence.current = String(sequence);
    setSelection({ key: '', manual: false });
    setCommands([]); setSearchFailures(0);
    if (!connected || !hasQuery || manage || active) { setSearching(false); return; }
    setSearching(true);
    void StartSearch(sequence, query).catch(() => {
      if (activeSequence.current === String(sequence)) { setSearching(false); setError('搜索请求失败，请重试。'); }
    });
    return () => { activeSequence.current = ''; void CancelSearch(sequence).catch(() => {}); };
  }, [query, connected, manage, active, data]);

  const ActiveView = active?.command.view ? pluginViews[`${active.plugin.id}/${active.command.view}`] : undefined;

  const selected = Math.max(0, commands.findIndex(item => item.key === selection.key));
  useEffect(() => {
    commandRefs.current[selected]?.scrollIntoView({ block: 'nearest' });
  }, [selected, commands]);

  function moveSelection(direction: number) {
    setSelection(current => {
      if (!commands.length) return current;
      const index = Math.max(0, commands.findIndex(item => item.key === current.key));
      return { key: commands[Math.max(0, Math.min(commands.length - 1, index + direction))].key, manual: true };
    });
  }

  async function perform(action: () => Promise<unknown>) {
    const current = generation.current;
    setBusy(true); setError('');
    try {
      await action();
      if (current === generation.current) await reload();
    } catch (e) {
      if (current === generation.current) setError(e instanceof Error ? e.message : '操作失败，请重试。');
    } finally {
      if (current === generation.current) setBusy(false);
    }
  }

  function inspectPackage() {
    const current = generation.current;
    setPreview(undefined); setTrust(false);
    void perform(async () => { const result = await PreviewPackage(directory); if (current === generation.current) setPreview(result); });
  }

  function choose(index: number) {
    const item = commands[index];
    if (!item || busy) return;
    const plugin = data.plugins.find(plugin => plugin.id === item.pluginId);
    const command = plugin?.commands.find(command => command.id === item.commandId);
    if (plugin && command?.view && !pluginViews[`${plugin.id}/${command.view}`]) { setError('此视图尚未在桌面宿主中注册。'); return; }
    if (plugin && command?.execution === 'direct') {
      const current = generation.current;
      void perform(async () => {
        const result = await Execute(plugin.id, command.id, item.input);
        if (current !== generation.current) return;
        if (result.dismiss) await HideLauncher();
        else setOutput(result.text);
      });
      return;
    }
    if (plugin && command) { setActive({ plugin, command }); setInput(item.input); setOutput(''); setError(''); }
  }

  return <main ref={shellRef} className="shell"
    onPointerEnter={event => { pointerPosition.current = { x: event.screenX, y: event.screenY }; }}
    onPointerMove={event => {
      const previous = pointerPosition.current;
      const position = { x: event.screenX, y: event.screenY };
      pointerPosition.current = position;
      if (previous?.x === position.x && previous.y === position.y) return;
      // Layout changes can move rows beneath a stationary pointer. Only actual
      // pointer movement should take selection back from keyboard navigation.
      const row = (event.target as Element).closest<HTMLButtonElement>('[data-search-key]');
      if (row?.dataset.searchKey) setSelection({ key: row.dataset.searchKey, manual: true });
    }}>
    {manage ? <section className="management">
      <div className="view-header"><button className="back" onClick={() => setManage(false)} aria-label="返回搜索">←</button><h1>本地插件</h1><button disabled={busy || !connected} onClick={() => perform(reload)}>刷新</button></div>
      <div className="management-body">
        <div className="plugins">
          {data.plugins.map(plugin => <div className="plugin" key={plugin.id}>
            <div><strong>{plugin.name}</strong><small>{plugin.version} · {states[plugin.state] ?? plugin.state}{plugin.changed ? ' · 文件有变更' : ''}</small></div>
            <div className="plugin-actions"><button disabled={busy || !connected || !['running', 'activated', 'failed'].includes(plugin.state)} onClick={() => perform(() => Stop(plugin.id))}>停止</button><button className="danger" disabled={busy || !connected} onClick={() => perform(async () => { await Uninstall(plugin.id); if (active?.plugin.id === plugin.id) setActive(undefined); })}>卸载</button><button disabled={busy || !connected} aria-label={`重载 ${plugin.name}`} onClick={() => perform(() => Reload(plugin.id))}>重载</button></div>
          </div>)}
          {!data.plugins.length && <p className="empty">还没有插件，从本地目录安装一个。</p>}
        </div>
        <form className="install" onSubmit={event => {
          event.preventDefault();
          const current = generation.current;
          if (!preview) {
            inspectPackage();
          } else if (preview.changed && (preview.kind !== 'process' || trust)) {
            void perform(async () => {
              try { await ApplyPackage(directory, trust, preview.fingerprint, preview.installedFingerprint); if (current === generation.current) setDirectory(''); }
              finally { if (current === generation.current) { setPreview(undefined); setTrust(false); } }
            });
          }
        }}>
          <label htmlFor="directory">安装 / 升级插件</label>
          <div className="install-row"><input id="directory" value={directory} disabled={busy} onChange={event => { setDirectory(event.target.value); setPreview(undefined); setTrust(false); setError(''); }} placeholder="插件目录的绝对路径" required /><button className="primary" disabled={busy || !connected || !directory.trim() || !!preview && (!preview.changed || preview.kind === 'process' && !trust)}>{!preview ? '检查' : !preview.changed ? '无需升级' : preview.installedVersion ? '升级' : '安装'}</button></div>
          {preview && <div className="package-preview" role="status"><strong>{preview.name}</strong><span>{preview.installedVersion ? `${preview.installedVersion} → ${preview.version}` : preview.version}</span><button type="button" disabled={busy} onClick={inspectPackage}>重新检查</button></div>}
          {preview?.kind === 'process' && preview.changed && <label className="trust"><input type="checkbox" checked={trust} onChange={event => setTrust(event.target.checked)} />信任此原生插件，允许其以当前用户权限运行</label>}
        </form>
        {data.diagnostics.map((diagnostic, index) => <p className="error" key={index}>{diagnostic}</p>)}
        {shortcutError && <p className="error">{shortcutError}</p>}
      </div>
    </section> : active ? <section className="tool">
      <div className="view-header"><button className="back" aria-label="返回搜索" onClick={() => { generation.current++; setBusy(false); setActive(undefined); setOutput(''); setError(''); }}>←</button><h1>{active.command.title}</h1>{active.plugin.name !== active.command.title && <span className="source">{active.plugin.name}</span>}</div>
      <>{ActiveView ? <ActiveView plugins={data.plugins} bridge={settingsBridge} /> : <div className="tool-body">
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
      </div>}</>
    </section> : <section className="launcher">
      <div className="search">
        <svg className="compass" viewBox="0 0 24 24" aria-hidden="true"><circle cx="11.2" cy="11.2" r="8.2"/><path d="m18.2 18.2 2.4 2.4"/><g transform="translate(11.2 11.2) rotate(45)"><path className="compass-tip" d="M0-6.5-2.8.5 0-1 2.8.5Z"/><path className="compass-tail" d="M-2.3 1.7 0 .3 2.3 1.7 0 6.5Z"/></g></svg>
        <input ref={searchRef} autoFocus aria-label="搜索命令" role="combobox" aria-autocomplete="list" aria-expanded={hasQuery} aria-controls="search-results" aria-activedescendant={hasQuery && commands[selected] ? `search-result-${selected}` : undefined} placeholder="搜索…" value={query} onChange={event => { setQuery(event.target.value); setSelection({ key: '', manual: false }); setOutput(''); setError(''); }} onKeyDown={event => {
          if (event.nativeEvent.isComposing) return;
          if (event.key === 'ArrowDown') { event.preventDefault(); moveSelection(1); }
          if (event.key === 'ArrowUp') { event.preventDefault(); moveSelection(-1); }
          if (event.key === 'Enter' && hasQuery) { event.preventDefault(); choose(selected); }
        }} />
        <button className="manage-button" aria-label="管理插件" title="管理插件" onClick={() => { setManage(true); setError(''); }}>···{shortcutError && <span className="warning-dot" />}</button>
      </div>
      {hasQuery && <>
        <div id="search-results" className="command-list" role="listbox" aria-label="搜索结果">
          {commands.map((item, index) => <button ref={element => { commandRefs.current[index] = element; }} className={`command ${index === selected ? 'selected' : ''}`} key={item.key} data-search-key={item.key} id={`search-result-${index}`} role="option" aria-selected={index === selected} onClick={() => choose(index)}>
            <span className="command-icon">{item.icon ? <img src={item.icon} alt="" /> : item.kind === 'process' ? '↔' : '✧'}</span><span><strong>{item.title}</strong><small>{item.subtitle}</small></span><span className="open">↵</span>
          </button>)}
          {!commands.length && <div className="empty">{searching ? '搜索中…' : '没有匹配的命令'}</div>}
        </div>
        <footer><span>{commands.length} 个结果{searching ? ' · 搜索中…' : searchFailures ? ` · ${searchFailures} 个来源暂不可用` : ''}</span><span>↑ ↓ 选择　↵ 打开　esc 隐藏</span></footer>
      </>}
    </section>}
    {!active && output && <output aria-live="polite">{output}</output>}
    {error && <p className="error" role="alert">{error}</p>}
  </main>;
}
