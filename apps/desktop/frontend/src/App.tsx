import PluginIcon from '../../../../packages/ui-components/PluginIcon';
import { useCallback, useEffect, useRef, useState } from 'react';
import { Events } from '@wailsio/runtime';
import { disposePaste, escapeAction, filterPasteResults, MAX_PASTE_TEXT_BYTES, pasteBackspaceText, readPastedImage } from './paste';
import type { PastedInput } from './paste';
import type { ClipboardEvent } from 'react';
import { pluginViews, settingsBridge } from './pluginViews';
import {UpdateNotice} from '../../../../plugins/system-settings/frontend/Updates';
import { keyCombination, formatShortcut } from '../../../../plugins/system-settings/frontend/Shortcut';
import Recovery from './Recovery';
import { isSearchOpenKey } from './searchKeyboard';
import PluginViewBoundary from './PluginViewBoundary';
import type { SettingsSchema } from '../../../../plugins/system-settings/frontend/Settings';
import { Execute, LauncherStatus, Refresh, ResizeLauncher, Snapshot, StartSearch, CancelSearch, HideLauncher, OpenSettings, OpenPluginCommand, OpenPluginCommandWithInput, DetachShortcutSettings, DetachTrustedView, CloseTrustedView, ClosePluginView, StartPasteSearch, ClearPaste, OpenPasteCommand, ResetToSearch } from '../bindings/sinan/apps/desktop/hostservice';

type Command = { uiView?: string; view?: string; id: string; title: string; keywords?: string[] | null; execution?: string };
type Plugin = { icon?: string; disabled?: boolean; changed?: boolean; id: string; name: string; version: string; kind: string; state: string; settings?: SettingsSchema[] | null; commands: Command[] };
type SearchItem = { pasteId?: string; icon?: string; key: string; pluginId: string; providerId: string; commandId: string; input: string; title: string; subtitle: string; kind: string; score: number };
type SearchUpdate = { queryId: string; items: SearchItem[]; pending: number; failures: string[]; done: boolean };
type Data = { plugins: Plugin[]; diagnostics: string[] };
function normalize(snapshot: { plugins: (Omit<Plugin, 'commands'> & { commands: Command[] | null })[] | null; diagnostics: string[] | null }): Data {
  return { plugins: (snapshot.plugins ?? []).map(plugin => ({ ...plugin, commands: plugin.commands ?? [] })), diagnostics: snapshot.diagnostics ?? [] };
}

export default function App() {
  const [data, setData] = useState<Data>({ plugins: [], diagnostics: [] });
  const [spaceToOpen, setSpaceToOpen] = useState(false);
  const searchPreferencesRevision = useRef(-1);
  const [query, setQuery] = useState('');
  const [pasted, setPasted] = useState<PastedInput>();
  const [pasteReading, setPasteReading] = useState(false);
  const pastedRef = useRef<PastedInput>();
  const pasteLoading = useRef(0);
  const pasteSearchSequence = useRef(0);
  const escapeRef = useRef<() => void>(() => {});
  const [commands, setCommands] = useState<SearchItem[]>([]);
  const [searching, setSearching] = useState(false);
  const [searchFailures, setSearchFailures] = useState(0);
  const searchSequence = useRef(Date.now() * 1000);
  const activeSequence = useRef('');
  const [selection, setSelection] = useState({ key: '', manual: false });
  const latestPlugins = useRef(data.plugins);latestPlugins.current=data.plugins;
  const pointerPosition = useRef<{ x: number; y: number }>();
  const [active, setActive] = useState<{ plugin: Plugin; command: Command }>();
  const [input, setInput] = useState('');
  const [output, setOutput] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [detached, setDetached] = useState(false);
  const [detachShortcut, setDetachShortcut] = useState(/mac/i.test(navigator.platform) ? 'Shift+Cmd+D' : 'Ctrl+Shift+D');
  const detachedRef = useRef(false);
  const detachRevision = useRef(-1);
  const detaching = useRef(false);
  const [recovery, setRecovery] = useState(false);
  const [connected, setConnected] = useState(false);
  const [shortcutError, setShortcutError] = useState('');
  const shellRef = useRef<HTMLElement>(null);
  const searchRef = useRef<HTMLInputElement>(null);
  const commandRefs = useRef<(HTMLButtonElement | null)[]>([]);
  // Invalidates late UI results when a running command is dismissed and reopened.
  const generation = useRef(0);
  const initialAcceptance = useRef(new URLSearchParams(window.location.search).get('acceptance') === '1');

  async function reload() {
    const snapshot = await Refresh();
    setData(normalize(snapshot));
    setConnected(true);
  }

  useEffect(() => {
    let alive = true;
    const receive = (state: {spaceToOpen: boolean; revision: number; error?: string}) => {
      if (!alive || state.revision < searchPreferencesRevision.current) return;
      searchPreferencesRevision.current = state.revision;
      setSpaceToOpen(!state.error && state.spaceToOpen);
    };
    const off = settingsBridge.search.subscribe(receive);
    void settingsBridge.search.load().then(receive).catch(() => {});
    return () => { alive = false; off(); };
  }, []);

  useEffect(() => {
    Snapshot().then(snapshot => { setData(normalize(snapshot)); setConnected(true); })
      .catch(() => setError('无法连接桌面宿主，请重新启动司南。'));
    LauncherStatus().then(setShortcutError).catch(() => {});
    const reset = () => {
      // This listener is registered once; read the current role synchronously.
      // A stale/broadcast launcher event must never reset a detached tool.
      if (detachedRef.current) return;
      generation.current++;
      pasteLoading.current++;
      disposePaste(pastedRef.current);pastedRef.current=undefined;setPasted(undefined);setPasteReading(false);
      void ClearPaste(pasteSearchSequence.current).catch(()=>{});
      activeSequence.current='';setSearchItems([]);setCommands([]);setSearching(false);setSearchFailures(0);
      setRecovery(false); setActive(undefined); setQuery(''); setSelection({ key: '', manual: false });
      setInput(''); setOutput(''); setError(''); setBusy(false);
      void LauncherStatus().then(setShortcutError).catch(() => {});
      void reload().catch(() => setError('无法读取插件，请重试。'));
      requestAnimationFrame(() => searchRef.current?.focus());
    };
    const offShow = Events.On('launcher:show', reset);
    const offReset = Events.On('launcher:reset', reset);
    const offPlugin = Events.On('plugin-view:open', event => {
      const view = event.data as {pluginId:string;viewId:string;commandId:string;title:string};
      const current=++generation.current;
      void Snapshot().then(snapshot=>{
        if(current!==generation.current)return;
        const next=normalize(snapshot),plugin=next.plugins.find(item=>item.id===view.pluginId&&!item.disabled);
        if(!plugin)return;
        setData(next);setRecovery(false);setBusy(false);setOutput('');setError('');
        setActive({plugin,command:{id:view.commandId,title:view.title||plugin.name,uiView:view.viewId}});
      }).catch(()=>{if(current===generation.current){void ClosePluginView();setBusy(false);setError('无法显示插件页面，请重试。');}});
    });
    const offPluginClosed = Events.On('plugin-view:closed', () => {
      generation.current++;
      if(detachedRef.current){void CloseTrustedView();return;}
      setActive(undefined);setBusy(false);setOutput('');
      requestAnimationFrame(()=>searchRef.current?.focus());
    });
    const offSettings = Events.On('settings:open', () => {
      if (detachedRef.current) return;
      const current = ++generation.current;
      void Snapshot().then(snapshot => {
        if (current !== generation.current || detachedRef.current) return;
        const next = normalize(snapshot);
        setData(next); setConnected(true);
        const plugin = next.plugins.find(plugin => plugin.id === 'system.settings' && !plugin.disabled);
        const command = plugin?.commands.find(command => command.id === 'open' && command.view === 'settings');
        if (!plugin || !command) { setError('设置插件不可用。'); return; }
        setRecovery(false); setActive({plugin, command}); setBusy(false); setOutput(''); setError('');
      }).catch(() => { if (current === generation.current) setError('无法读取设置插件。'); });
    });
    const applyDetachShortcut = (state: {active:string; shortcut:string; revision:number}) => {
      if (state.revision < detachRevision.current) return;
      detachRevision.current = state.revision;
      setDetachShortcut(state.active || state.shortcut);
    };
    void DetachShortcutSettings().then(applyDetachShortcut).catch(() => {});
    const offDetachShortcut = Events.On('detach-shortcut:changed', event => applyDetachShortcut(event.data as {active:string;shortcut:string;revision:number}));
    const offDetached = Events.On('view:detached', () => { detachedRef.current = true; setDetached(true); });
    const offPluginShortcutError = Events.On('plugin-shortcut:error', event => setError(String(event.data)));
    const offShortcut = Events.On('shortcut:changed', event => setShortcutError((event.data as {error?: string}).error ?? ''));
    return () => { offShow(); offReset(); offPlugin(); offPluginClosed(); offSettings(); offShortcut(); offDetached(); offDetachShortcut(); offPluginShortcutError(); };
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

  const hasQuery = query.trim().length > 0 || !!pasted;
  const [searchItems, setSearchItems] = useState<SearchItem[]>([]);
  useEffect(() => {
    if (!pasted) return;
    const next=filterPasteResults(searchItems, query, data.plugins);
    setCommands(next);setSelection(current=>current.manual && next.some(item=>item.key===current.key) ? current : {key:next[0]?.key ?? '',manual:false});
  }, [pasted, searchItems, query, data]);
  useEffect(() => Events.On('search:update', event => {
    const update = event.data as SearchUpdate;
    if (update.queryId !== activeSequence.current) return;
    const next = update.items ?? [];
    setSearchItems(next);setCommands(pastedRef.current ? filterPasteResults(next, searchRef.current?.value ?? '', latestPlugins.current) : next);
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
    if (!connected || !hasQuery || recovery || active) { setSearching(false); return; }
    setSearching(true);
    const request = pasted ? (() => {
      pasteSearchSequence.current=sequence;
      return StartPasteSearch(sequence, pasted.kind==='text' ? {text:pasted.text} : {image:{dataBase64:pasted.dataBase64}});
    })() : StartSearch(sequence, query);
    void request.catch((reason: unknown) => {
      if (activeSequence.current === String(sequence)) { setSearching(false); setError(reason instanceof Error ? reason.message : '搜索请求失败，请重试。'); }
    });
    return () => { activeSequence.current = ''; void CancelSearch(sequence).catch(() => {}); };
  }, [pasted ? 'paste' : query, pasted, connected, recovery, active, data]);

  function clearPasteState() {
    pasteLoading.current++;disposePaste(pastedRef.current);pastedRef.current=undefined;setPasted(undefined);setPasteReading(false);
    setSearchItems([]);setCommands([]);activeSequence.current='';
    void ClearPaste(pasteSearchSequence.current).catch(()=>{});
  }
  function backspacePaste() {
    if (!pasted) return;
    const text=pasteBackspaceText(pasted);
    clearPasteState();setQuery(text);setSelection({key:'',manual:false});setOutput('');setError('');
    requestAnimationFrame(()=>searchRef.current?.focus());
  }
  function resetShell() {
    generation.current++;clearPasteState();
    setActive(undefined);setRecovery(false);setQuery('');setInput('');setOutput('');setError('');setBusy(false);
    setSelection({key:'',manual:false});setSearching(false);setSearchFailures(0);
    requestAnimationFrame(()=>searchRef.current?.focus());
  }
  escapeRef.current=()=>{
    if (escapeAction(!!active || !!pasted || pasteReading || recovery || detached || busy || !!output,query)==='hide') { void HideLauncher();return; }
    if (!detached) resetShell();
    void ResetToSearch().catch(()=>setError('无法返回主输入框，请重试。'));
  };
  useEffect(()=>{
    const escape=(event:KeyboardEvent)=>{
      if(event.key!=='Escape' || event.repeat || event.isComposing)return;
      event.preventDefault();event.stopPropagation();escapeRef.current();
    };
    window.addEventListener('keydown',escape,true);
    return ()=>{window.removeEventListener('keydown',escape,true);disposePaste(pastedRef.current);};
  },[]);
  async function handlePaste(event: ClipboardEvent<HTMLInputElement>) {
    if (query.length || pasted || active || detached) return;
    const image=Array.from(event.clipboardData.items).find(item=>item.kind==='file' && item.type.startsWith('image/'))?.getAsFile();
    const text=event.clipboardData.getData('text/plain');
    if(!image && !text)return;
    event.preventDefault();
    const ticket=++pasteLoading.current;setPasteReading(true);
    const openedAt=generation.current;
    try {
      if (!image && new TextEncoder().encode(text).length>MAX_PASTE_TEXT_BYTES) throw new Error('粘贴文本超过 64 KiB');
      const next:PastedInput=image ? await readPastedImage(image) : {kind:'text',text};
      if(ticket!==pasteLoading.current || openedAt!==generation.current || searchRef.current?.value) {disposePaste(next);return;}
      setSearchItems([]);setCommands([]);pastedRef.current=next;setPasted(next);setQuery('');setOutput('');setError('');setSelection({key:'',manual:false});
    } catch(reason) {if(ticket===pasteLoading.current)setError(reason instanceof Error ? reason.message : '无法读取粘贴内容');}
    finally {if(ticket===pasteLoading.current)setPasteReading(false);}
  }

  const ActiveView = active?.command.view ? pluginViews[`${active.plugin.id}/${active.command.view}`] : undefined;
  async function detachView() {
    if (!active || (!ActiveView && !active.command.uiView) || detached || detaching.current) return;
    detaching.current = true;
    setError('');
    try {
      await DetachTrustedView(active.plugin.id, active.command.id);
      detachedRef.current = true; setDetached(true);
    } catch { setError('拆出失败，请重试。'); }
    finally { detaching.current = false; }
  }
  useEffect(() => {
    const keydown = (event: KeyboardEvent) => {
      const primary = /mac/i.test(navigator.platform) ? event.metaKey && !event.ctrlKey : event.ctrlKey && !event.metaKey;
      if (!event.defaultPrevented && !event.isComposing && !event.repeat && primary && !event.altKey && !event.shiftKey && event.code === 'Comma') {
        event.preventDefault();
        if (active?.plugin.id !== 'system.settings') void OpenSettings().catch(() => setError('无法打开设置。'));
        return;
      }
      if (event.defaultPrevented || event.isComposing || event.repeat || keyCombination(event) !== detachShortcut || !ActiveView || detached) return;
      event.preventDefault();
      void detachView();
    };
    window.addEventListener('keydown', keydown);
    return () => window.removeEventListener('keydown', keydown);
  }, [active, ActiveView, detached, detachShortcut]);


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

  const syncPlugins = useCallback(async () => {
    setData(normalize(await Snapshot()));
  }, []);
  const installedManager = data.plugins.find(plugin => plugin.id === 'system.plugin-manager');
  const managerPlugin = installedManager?.disabled ? undefined : installedManager;
  const managerCommand = managerPlugin?.commands.find(command => command.id === 'open' && command.view === 'manager');
  function openRecovery() {
    if (detached) { void CloseTrustedView(); return; }
    generation.current++; setActive(undefined); setRecovery(true); setBusy(false); setOutput(''); setError('');
  }
  function openManager() {
    if (!managerPlugin || !managerCommand) { openRecovery(); return; }
    generation.current++; setRecovery(false); setActive({plugin:managerPlugin,command:managerCommand}); setBusy(false); setOutput(''); setError('');
  }
  useEffect(() => {
    if (connected && initialAcceptance.current) {
      initialAcceptance.current = false;
      openManager();
    }
  }, [connected, managerPlugin, managerCommand]);
  useEffect(() => {
    if (active?.command.view && !data.plugins.some(plugin => !plugin.disabled && plugin.id === active.plugin.id && plugin.commands.some(command => command.id === active.command.id && command.view === active.command.view))) {
      if (detached) { void CloseTrustedView(); return; }
      generation.current++; setActive(undefined); setBusy(false); setOutput('');
      if (active.plugin.id === 'system.plugin-manager') setRecovery(true);
    }
  }, [data, active, detached]);

  function choose(index: number) {
    const item = commands[index];
    if (!item || busy) return;
    const plugin = data.plugins.find(plugin => plugin.id === item.pluginId);
    if(plugin?.disabled){setError('插件已禁用。');return;}
    const command = plugin?.commands.find(command => command.id === item.commandId);
    if (plugin && command?.uiView) {
      const current = generation.current;
      void perform(async () => {
        const embedded = await (item.pasteId ? OpenPasteCommand(plugin.id,command.id,item.pasteId) : item.input ? OpenPluginCommandWithInput(plugin.id, command.id, item.input) : OpenPluginCommand(plugin.id, command.id));
        if (!embedded && current === generation.current) await HideLauncher();
      });
      return;
    }
    if (plugin && command?.view && !pluginViews[`${plugin.id}/${command.view}`]) { setError('此视图尚未在桌面宿主中注册。'); return; }
    if (plugin && command?.execution === 'direct') {
      const current = generation.current;
      void perform(async () => {
        const result = await Execute(plugin.id, command.id, pasted?.kind==='text' ? pasted.text : item.input);
        if (current !== generation.current) return;
        if (result.dismiss) await HideLauncher();
        else setOutput(result.text);
      });
      return;
    }
    if (plugin && command) { setActive({ plugin, command }); setInput(pasted?.kind==='text' ? pasted.text : item.input); setOutput(''); setError(''); }
  }

  return <main ref={shellRef} className={`shell${active || recovery ? " plugin-view" : ""}${detached ? " detached" : ""}`}
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
    {recovery ? <section>
      <div className="view-header"><button className="back" onClick={() => { setRecovery(false); setError(''); }} aria-label="返回搜索">←</button><h1>恢复入口</h1></div>
      <Recovery available={!!managerCommand} disabled={!!installedManager?.disabled} diagnostics={data.diagnostics} shortcutError={shortcutError} onRefresh={reload} onOpen={openManager}/>
    </section> : active ? <section className="tool">
      <div className="view-header"><button className="back" aria-label={detached ? "关闭窗口" : "返回搜索"} onClick={() => { if (detached) { void CloseTrustedView(); return; } void (async()=>{if(active.command.uiView)await ClosePluginView();generation.current++;setBusy(false);setActive(undefined);setOutput('');setError('');requestAnimationFrame(()=>searchRef.current?.focus());})(); }}>{detached ? '×' : '←'}</button><PluginIcon name={active.plugin.name} icon={active.plugin.icon} className="header-plugin-icon"/><h1>{active.command.title}</h1>{active.plugin.name !== active.command.title && <span className="source">{active.plugin.name}</span>}{(ActiveView || active.command.uiView) && !detached && <button className="detach-button" aria-label="独立窗口" title={`独立窗口（${formatShortcut(detachShortcut)}）`} onClick={() => void detachView()}>↗</button>}</div>
      <>{active.command.uiView ? <div className="embedded-plugin-slot" aria-label="插件内容"/> : ActiveView ? <PluginViewBoundary key={`${active.plugin.id}/${active.command.id}`} onRecover={openRecovery}><ActiveView plugins={data.plugins} onChanged={syncPlugins} onRecover={openRecovery} /></PluginViewBoundary> : <div className="tool-body">
        <form onSubmit={event => {
          event.preventDefault();
          const current = generation.current;
          setOutput('');
          void perform(async () => {
            const result = await Execute(active.plugin.id, active.command.id, input);
            if (current === generation.current) setOutput(result.text);
          });
        }}>
          {active.plugin.kind === 'process' && <textarea autoFocus spellCheck={false} autoCorrect="off" autoCapitalize="off" autoComplete="off" aria-label="命令输入" placeholder="输入一段文字…" value={input} onChange={event => setInput(event.target.value)} maxLength={16000} />}
          <button className="primary" disabled={busy || !connected}>{busy ? '执行中…' : '执行'}</button>
        </form>
        {output && <output aria-live="polite">{output}</output>}
      </div>}</>
    </section> : <section className="launcher">
      <div className="search">
        {pasted ? <div className={`paste-preview ${pasted.kind}`} aria-label={pasted.kind==='image' ? '粘贴图片预览' : '粘贴文本预览'}>
          {pasted.kind==='image' ? <img src={pasted.previewURL} alt="粘贴的图片" /> : <span>{pasted.text.replace(/\s+/g,' ').trim().slice(0,160)}</span>}
        </div> : <svg className={`compass${searching ? ' searching' : ''}`} viewBox="0 0 24 24" aria-hidden="true"><circle cx="11.2" cy="11.2" r="8.2"/><path d="m18.2 18.2 2.4 2.4"/><g className="compass-needle" transform="translate(11.2 11.2) rotate(45)"><path className="compass-tip" d="M0-6.5-2.8.5 0-1 2.8.5Z"/><path className="compass-tail" d="M-2.3 1.7 0 .3 2.3 1.7 0 6.5Z"/></g></svg>}
        <input ref={searchRef} autoFocus spellCheck={false} autoCorrect="off" autoCapitalize="off" autoComplete="off" aria-label="搜索命令" role="combobox" aria-autocomplete="list" aria-expanded={hasQuery} aria-controls="search-results" aria-activedescendant={hasQuery && commands[selected] ? `search-result-${selected}` : undefined} placeholder={pasted ? "筛选结果…" : pasteReading ? "正在读取图片…" : "搜索…"} onPaste={event=>void handlePaste(event)} value={query} onChange={event => { pasteLoading.current++;setPasteReading(false);setQuery(event.target.value); setSelection({ key: '', manual: false }); setOutput(''); setError(''); }} onKeyDown={event => {
          if (event.nativeEvent.isComposing || event.nativeEvent.keyCode === 229) return;
          if (event.key === 'Backspace' && pasted && query.length===0) {event.preventDefault();backspacePaste();return;}
          if (event.key === 'ArrowDown') { event.preventDefault(); moveSelection(1); }
          if (event.key === 'ArrowUp') { event.preventDefault(); moveSelection(-1); }
          if (isSearchOpenKey(event.nativeEvent, spaceToOpen) && hasQuery) { event.preventDefault(); choose(selected); }
        }} />
        <button className="manage-button" aria-label="管理插件" title="管理插件" onClick={openManager}>···{shortcutError && <span className="warning-dot" />}</button>
        <UpdateNotice bridge={settingsBridge.updates} onOpen={()=>void OpenSettings().catch(()=>setError('无法打开设置。'))}/>
      </div>
      {hasQuery && <>
        <div id="search-results" className="command-list" role="listbox" aria-label="搜索结果">
          {commands.map((item, index) => <button ref={element => { commandRefs.current[index] = element; }} className={`command ${index === selected ? 'selected' : ''}`} key={item.key} data-search-key={item.key} id={`search-result-${index}`} role="option" aria-selected={index === selected} onClick={() => choose(index)}>
            <PluginIcon name={data.plugins.find(plugin=>plugin.id===item.pluginId)?.name??item.title} icon={item.icon||data.plugins.find(plugin=>plugin.id===item.pluginId)?.icon} className="command-icon"/><span><strong>{item.title}</strong><small>{item.subtitle}</small></span><span className="open">↵</span>
          </button>)}
          {!commands.length && <div className="empty">{searching ? '搜索中…' : '没有匹配的命令'}</div>}
        </div>
        <footer><span>{commands.length} 个结果{searching ? ' · 搜索中…' : searchFailures ? ` · ${searchFailures} 个来源暂不可用` : ''}</span><span>↑ ↓ 选择　{spaceToOpen ? '↵ / 空格' : '↵'} 打开　esc 返回 / 隐藏</span></footer>
      </>}
    </section>}
    {!active && output && <output aria-live="polite">{output}</output>}
    {error && <p className="error" role="alert">{error}</p>}
  </main>;
}
