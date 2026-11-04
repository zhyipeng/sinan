// Only repository-bundled trusted components can appear in the Shell.
// A manifest view key never becomes an import URL or installed JavaScript path.
import Settings, { type SettingsBridge, type SettingsSnapshot } from '../../../../plugins/system-settings/frontend/Settings';
import { PluginSettings, SavePluginSettings, ClearHistory } from '../bindings/sinan/apps/desktop/hostservice';
export const pluginViews: Record<string, typeof Settings | undefined> = { 'system.settings/settings': Settings };
function normalize(value: {revision: number; values: Record<string, string[] | null | undefined> | null}): SettingsSnapshot {
  return { revision: value.revision, values: Object.fromEntries(Object.entries(value.values ?? {}).map(([key, paths]) => [key, paths ?? []])) };
}
export const settingsBridge: SettingsBridge = {
  async load(pluginID) { return normalize(await PluginSettings(pluginID)); },
  async save(pluginID, revision, values) { return normalize(await SavePluginSettings(pluginID, revision, values)); },
  async clearHistory() { await ClearHistory(); },
};
