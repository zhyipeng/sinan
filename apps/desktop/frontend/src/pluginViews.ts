// Only repository-bundled trusted components can appear in the Shell.
// A manifest view key never becomes an import URL or installed JavaScript path.
import { Events } from '@wailsio/runtime';
import { createElement, type ComponentType } from 'react';
import PluginManager, {type ManagementBridge} from '../../../../plugins/system-plugin-manager/frontend/PluginManager';
import type { ShortcutState } from '../../../../plugins/system-settings/frontend/Shortcut';
import Settings, { type ConfigurablePlugin, type SettingsBridge, type SettingsSnapshot } from '../../../../plugins/system-settings/frontend/Settings';
import { PluginSettings, SavePluginSettings, ClearHistory, ShortcutSettings, SaveShortcut, RecordShortcut, Refresh, Stop, Uninstall, Reload, PreviewPackage, ApplyPackage, SetPluginEnabled, OpenPluginView, PluginUIEnabled, DetachShortcutSettings, SaveDetachShortcut, PluginData, ClearPluginData } from '../bindings/sinan/apps/desktop/hostservice';
type ViewProps = {plugins: ConfigurablePlugin[]; onChanged():Promise<void>; onRecover():void};
export const pluginViews: Record<string, ComponentType<ViewProps> | undefined> = {
  'system.settings/settings': ({plugins}) => createElement(Settings,{plugins,bridge:settingsBridge}),
  'system.plugin-manager/manager': ({onChanged,onRecover}) => createElement(PluginManager,{bridge:managementBridge,onChanged,onRecover}),
};
// These adapters are private to bundled trusted views, never passed to installed JS.
const managementBridge: ManagementBridge = {
  async listData(){return await PluginData()??[];},
  async clearData(id,token){await ClearPluginData(id,token);},
  async refresh(){const [data,uiEnabled]=await Promise.all([Refresh(),PluginUIEnabled()]);return {plugins:data.plugins??[],diagnostics:data.diagnostics??[],uiEnabled};},
  async stop(id){await Stop(id);},
  async uninstall(id){await Uninstall(id);},
  async reload(id){return await Reload(id);},
  async inspect(directory){return await PreviewPackage(directory);},
  async apply(directory,trust,fingerprint,installedFingerprint){return await ApplyPackage(directory,trust,fingerprint,installedFingerprint);},
  async setEnabled(id,enabled){return await SetPluginEnabled(id,enabled);},
  async openView(id,viewID){await OpenPluginView(id,viewID);},
  subscribeWindows(failed){return Events.On('ui:closed',event=>{if((event.data as {failed?:boolean}).failed)failed();});},
};
function normalize(value: {revision: number; values: Record<string, string[] | null | undefined> | null}): SettingsSnapshot {
  return { revision: value.revision, values: Object.fromEntries(Object.entries(value.values ?? {}).map(([key, paths]) => [key, paths ?? []])) };
}
let recordingQueue: Promise<void> = Promise.resolve();
export const settingsBridge: SettingsBridge = {
  detachShortcut: {
    async load() { return await DetachShortcutSettings(); },
    async save(value, revision) { return await SaveDetachShortcut(value, revision); },
    record(enabled) { recordingQueue = recordingQueue.catch(() => {}).then(() => RecordShortcut(enabled)).then(() => {}); return recordingQueue; },
    subscribe(changed, captured) {
      const offState = Events.On('detach-shortcut:changed', event => changed(event.data as ShortcutState));
      const offCaptured = Events.On('shortcut:recorded', event => captured(event.data as string));
      return () => { offState(); offCaptured(); };
    },
  },
  shortcut: {
    async load() { return await ShortcutSettings(); },
    async save(value, revision) { return await SaveShortcut(value, revision); },
    record(enabled) { recordingQueue = recordingQueue.catch(() => {}).then(() => RecordShortcut(enabled)).then(() => {}); return recordingQueue; },
    subscribe(changed, captured) {
      const offState = Events.On('shortcut:changed', event => changed(event.data as ShortcutState));
      const offCaptured = Events.On('shortcut:recorded', event => captured(event.data as string));
      return () => { offState(); offCaptured(); };
    },
  },
  async load(pluginID) { return normalize(await PluginSettings(pluginID)); },
  async save(pluginID, revision, values) { return normalize(await SavePluginSettings(pluginID, revision, values)); },
  async clearHistory() { await ClearHistory(); },
};
