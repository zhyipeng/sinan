import { useEffect, useState } from 'react';
import Shortcut, {type ShortcutBridge} from './Shortcut';
import PluginShortcuts,{type PluginShortcutsBridge} from './PluginShortcuts';
import Theme,{type ThemeBridge} from './Theme';
import LLM,{type LLMBridge} from './LLM';
import Search, {type SearchBridge} from './Search';
import Updates,{type UpdateBridge} from './Updates';

export type SettingsSchema = { id: string; title: string; type: string; options?: { value: string; label: string }[] | null; placeholder?: string | null };
export type ConfigurablePlugin = { id: string; name: string; uiViews?: {permissions?:string[]|null}[]|null;settings?: SettingsSchema[] | null };
export type SettingsSnapshot = { revision: number; values: Record<string, string[]> };
export type SettingsBridge = {
  search:SearchBridge;
  llm:LLMBridge;
  updates:UpdateBridge;
  theme:ThemeBridge;
  shortcut: ShortcutBridge;
  detachShortcut: ShortcutBridge;
  pluginShortcuts: PluginShortcutsBridge;
  load(pluginID: string): Promise<SettingsSnapshot>;
  save(pluginID: string, revision: number, values: Record<string, string[]>): Promise<SettingsSnapshot>;
  clearHistory(): Promise<void>;
};

const categories = [
  { id: 'general', label: '通用', icon: 'M4 5h16M4 12h16M4 19h16M8 3v4M16 10v4M10 17v4' },
  { id: 'search', label: '搜索', icon: 'M21 21l-5-5M18 10a8 8 0 1 1-16 0 8 8 0 0 1 16 0' },
  { id: 'shortcuts', label: '快捷键', icon: 'M5 4h2a3 3 0 1 1-3 3V5a1 1 0 0 1 1-1Zm12 0h2a1 1 0 0 1 1 1v2a3 3 0 1 1-3-3ZM4 17a3 3 0 1 1 3 3H5a1 1 0 0 1-1-1Zm13 3a3 3 0 1 1 3-3v2a1 1 0 0 1-1 1ZM7 7h10v10H7Z' },
  { id: 'ai', label: 'AI 服务', icon: 'm12 3 2.5 6.5L21 12l-6.5 2.5L12 21l-2.5-6.5L3 12l6.5-2.5Z' },
  { id: 'plugins', label: '插件设置', icon: 'M8 3v4m8-4v4M6 7h12v4a6 6 0 0 1-12 0Zm6 10v4' },
  { id: 'about', label: '关于与更新', icon: 'M12 11v6m0-10v.01M22 12a10 10 0 1 1-20 0 10 10 0 0 1 20 0' },
] as const;
type Category = typeof categories[number]['id'];

type Draft = { revision: number; values: Record<string, string>; saved: Record<string, string> };

function toDraftValues(plugin: ConfigurablePlugin, values: Record<string, string[]>): Record<string, string> {
  return Object.fromEntries((plugin.settings ?? []).map(field => {
    const raw = values[field.id] ?? [];
    if (field.type === 'select') return [field.id, raw[0] ?? field.options?.[0]?.value ?? ''];
    if (field.type === 'text') return [field.id, raw[0] ?? ''];
    return [field.id, raw.join('\n')];
  }));
}
function fromDraftValues(plugin: ConfigurablePlugin, draft: Record<string, string>): Record<string, string[]> {
  return Object.fromEntries((plugin.settings ?? []).map(field => {
    const raw = draft[field.id] ?? '';
    if (field.type === 'directory-list') return [field.id, raw.split('\n').map(path => path.trim()).filter(Boolean)];
    if (field.type === 'select' || field.type === 'text') { const value = raw.trim(); return [field.id, value ? [value] : []]; }
    return [field.id, []];
  }));
}
export default function Settings({ plugins, bridge }: { plugins: ConfigurablePlugin[]; bridge: SettingsBridge }) {
  const [category, setCategory] = useState<Category>('general');
  const [drafts, setDrafts] = useState<Record<string, Draft>>({});
  const [loading, setLoading] = useState(true);
  const [pending, setPending] = useState('');
  const [notice, setNotice] = useState('');
  const [error, setError] = useState('');
  useEffect(() => {
    let current = true;
    setLoading(true);
    Promise.all(plugins.filter(plugin => plugin.settings?.length).map(async plugin => {
      const value = await bridge.load(plugin.id);
      const fields = toDraftValues(plugin, value.values);
      return [plugin.id, { revision: value.revision, values: fields, saved: fields }] as const;
    })).then(entries => { if (current) setDrafts(Object.fromEntries(entries)); })
      .catch(() => { if (current) setError('无法读取设置，请返回后重试。'); })
      .finally(() => { if (current) setLoading(false); });
    return () => { current = false; };
  }, [plugins, bridge]);

  async function save(plugin: ConfigurablePlugin) {
    const draft = drafts[plugin.id];
    if (!draft || pending) return;
    setPending(plugin.id); setNotice(''); setError('');
    try {
      const values = fromDraftValues(plugin, draft.values);
      const result = await bridge.save(plugin.id, draft.revision, values);
      const saved = toDraftValues(plugin, result.values);
      setDrafts(current => ({ ...current, [plugin.id]: { revision: result.revision, values: saved, saved } }));
      setNotice('已保存，下次请求生效。');
    } catch (error) { setError(error instanceof Error ? error.message : '保存失败，请重试。'); }
    finally { setPending(''); }
  }
  async function clear() {
    if (pending) return;
    setPending('history'); setError(''); setNotice('');
    try { await bridge.clearHistory(); setNotice('使用历史已清除。'); }
    catch { setError('清除失败，请检查本地存储后重试。'); }
    finally { setPending(''); }
  }

  const configurable = plugins.filter(plugin => plugin.settings?.length);
  function renderPluginSettings(plugin: ConfigurablePlugin) {
    const draft = drafts[plugin.id];
    return <form className="settings-section" key={plugin.id} onSubmit={event => { event.preventDefault(); void save(plugin); }}>
      <h2>{plugin.name}</h2>
      {(plugin.settings ?? []).map(field => {
        const update = (value: string) => {
          setDrafts(current => ({ ...current, [plugin.id]: { ...current[plugin.id], values: { ...current[plugin.id].values, [field.id]: value } } }));
          setNotice(''); setError('');
        };
        const common = { id: `${plugin.id}-${field.id}`, disabled: !draft || !!pending, value: draft?.values[field.id] ?? '' };
        return <div className="settings-field" key={field.id}>
          <label htmlFor={`${plugin.id}-${field.id}`}>{field.title}</label>
          {field.type === 'select'
            ? <select {...common} onChange={event => update(event.target.value)}>
                {(field.options ?? []).map(option => <option key={option.value} value={option.value}>{option.label}</option>)}
              </select>
            : field.type === 'text'
            ? <input {...common} type="text" maxLength={2048} spellCheck={false} autoCorrect="off" autoCapitalize="off" autoComplete="off" placeholder={field.placeholder ?? ''} onChange={event => update(event.target.value)} />
            : <textarea {...common} rows={3} maxLength={16400} spellCheck={false} autoCorrect="off" autoCapitalize="off" autoComplete="off" placeholder="每行一个目录绝对路径" onChange={event => update(event.target.value)} />}
        </div>;
      })}
      <div className="settings-save"><span className="settings-note">{plugin.id === 'system.app-search' ? '标准应用目录始终保留' : ''}</span><button className="primary" disabled={!draft || !!pending || !Object.keys(draft.values).some(key => draft.values[key] !== draft.saved[key])}>{pending === plugin.id ? '保存中…' : '保存'}</button></div>
    </form>;
  }

  return <div className="settings-body">
    <nav className="settings-navigation" aria-label="设置分类">
      {categories.map(item => <button key={item.id} type="button"
        className={`settings-navigation-item${item.id === 'about' ? ' settings-navigation-about' : ''}`}
        aria-current={category === item.id ? 'page' : undefined}
        aria-controls={`settings-panel-${item.id}`} onClick={() => setCategory(item.id)}>
        <svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true"><path d={item.icon} /></svg>
        <span>{item.label}</span>
      </button>)}
    </nav>
    {/* 保持各分类挂载，切换时保留表单草稿和各自的滚动位置。 */}
    <div className="settings-content">
      <section className="settings-panel" id="settings-panel-general" hidden={category !== 'general'} aria-labelledby="settings-heading-general">
        <header className="settings-panel-heading"><h1 id="settings-heading-general">通用</h1></header>
        <Theme bridge={bridge.theme} />
      </section>
      <section className="settings-panel" id="settings-panel-search" hidden={category !== 'search'} aria-labelledby="settings-heading-search">
        <header className="settings-panel-heading"><h1 id="settings-heading-search">搜索</h1></header>
        <Search bridge={bridge.search} />
        {loading && <p className="settings-note" role="status">正在读取设置…</p>}
        {configurable.filter(plugin => plugin.id === 'system.app-search').map(renderPluginSettings)}
        <section className="settings-history"><div><h2>使用历史</h2><p className="settings-note">用于调整搜索结果顺序</p></div><button disabled={!!pending} onClick={() => void clear()}>{pending === 'history' ? '清除中…' : '清除使用历史'}</button></section>
      </section>
      <section className="settings-panel" id="settings-panel-shortcuts" hidden={category !== 'shortcuts'} aria-labelledby="settings-heading-shortcuts">
        <header className="settings-panel-heading"><h1 id="settings-heading-shortcuts">快捷键</h1></header>
        <Shortcut bridge={bridge.shortcut} />
        <Shortcut bridge={bridge.detachShortcut} title="窗口独立快捷键" />
        <PluginShortcuts bridge={bridge.pluginShortcuts} />
      </section>
      <section className="settings-panel" id="settings-panel-ai" hidden={category !== 'ai'} aria-labelledby="settings-heading-ai">
        <header className="settings-panel-heading"><h1 id="settings-heading-ai">AI 服务</h1></header>
        <LLM bridge={bridge.llm} plugins={plugins.filter(p => p.uiViews?.some(v => v.permissions?.includes('llm')))} />
      </section>
      <section className="settings-panel" id="settings-panel-plugins" hidden={category !== 'plugins'} aria-labelledby="settings-heading-plugins">
        <header className="settings-panel-heading"><h1 id="settings-heading-plugins">插件设置</h1></header>
        {loading && <p className="settings-note" role="status">正在读取设置…</p>}
        {configurable.filter(plugin => plugin.id !== 'system.app-search').map(renderPluginSettings)}
        {!loading && !configurable.some(plugin => plugin.id !== 'system.app-search') && <p className="settings-empty">暂无可配置的插件</p>}
      </section>
      <section className="settings-panel" id="settings-panel-about" hidden={category !== 'about'} aria-labelledby="settings-heading-about">
        <header className="settings-panel-heading"><h1 id="settings-heading-about">关于与更新</h1></header>
        <Updates bridge={bridge.updates} />
      </section>
      {(notice || error) && <div className="settings-feedback">
        {notice && <p className="settings-notice" role="status">{notice}</p>}
        {error && <p className="error" role="alert">{error}</p>}
      </div>}
    </div>
  </div>;
}
