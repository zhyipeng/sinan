import { useEffect, useState } from 'react';
import Shortcut, {type ShortcutBridge} from './Shortcut';
import PluginShortcuts,{type PluginShortcutsBridge} from './PluginShortcuts';
import Theme,{type ThemeBridge} from './Theme';
import LLM,{type LLMBridge} from './LLM';
import Updates,{type UpdateBridge} from './Updates';

export type SettingsSchema = { id: string; title: string; type: string; options?: { value: string; label: string }[] | null; placeholder?: string | null };
export type ConfigurablePlugin = { id: string; name: string; uiViews?: {permissions?:string[]|null}[]|null;settings?: SettingsSchema[] | null };
export type SettingsSnapshot = { revision: number; values: Record<string, string[]> };
export type SettingsBridge = {
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

  return <div className="settings-body">
    {loading && <p className="settings-note">正在读取设置…</p>}
    {plugins.filter(plugin => plugin.settings?.length).map(plugin => {
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
    })}
    <section className="settings-history"><div><h2>使用历史</h2><p className="settings-note">用于调整搜索结果顺序</p></div><button disabled={!!pending} onClick={() => void clear()}>{pending === 'history' ? '清除中…' : '清除使用历史'}</button></section>
    <Shortcut bridge={bridge.shortcut} />
    <Shortcut bridge={bridge.detachShortcut} title="窗口独立快捷键" />
    <Theme bridge={bridge.theme} />
    <PluginShortcuts bridge={bridge.pluginShortcuts} />
    <Updates bridge={bridge.updates} />
    <LLM bridge={bridge.llm} plugins={plugins.filter(p=>p.uiViews?.some(v=>v.permissions?.includes("llm")))} />
    {notice && <p className="settings-notice" role="status">{notice}</p>}
    {error && <p className="error" role="alert">{error}</p>}
  </div>;
}
