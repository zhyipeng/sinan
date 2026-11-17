// Only repository-bundled trusted components can appear in the Shell.
// A manifest view key never becomes an import URL or installed JavaScript path.
import { Events } from '@wailsio/runtime';
import {UpdateStatus,CheckHostUpdate,InstallHostUpdate} from '../bindings/sinan/apps/desktop/hostservice';
import { createElement, type ComponentType } from 'react';
import type {StoreBridge} from '../../../../plugins/system-plugin-manager/frontend/PluginStore';
import PluginManager, {type ManagementBridge} from '../../../../plugins/system-plugin-manager/frontend/PluginManager';
import type { ShortcutState } from '../../../../plugins/system-settings/frontend/Shortcut';
import Settings, { type ConfigurablePlugin, type SettingsBridge, type SettingsSnapshot } from '../../../../plugins/system-settings/frontend/Settings';
import { WindowOpacitySettings, SaveWindowOpacity, StartupSettings, SaveSilentStartup, SaveAutoStart, SearchPreferences, SaveSearchPreferences, LLMSettings, SaveLLMProfile, DeleteLLMProfile, SaveLLMPreferences, PluginShortcutSettings, SavePluginShortcut, PluginSettings, SavePluginSettings, ClearHistory, ShortcutSettings, SaveShortcut, RecordShortcut, Refresh, Stop, Uninstall, Reload, PreviewPackage, ApplyPackage, SetPluginEnabled, OpenPluginView, PluginUIEnabled, DetachShortcutSettings, SaveDetachShortcut, PluginData, ClearPluginData, ThemeSettings, SaveTheme, ChoosePluginDirectory, CreatePluginProject, ChooseInstallDirectory, ChooseInstallArchive, TrustDevelopmentPlugin, StoreCatalog, StoreInstall, StoreInstallTasks, StoreCancelInstall } from '../bindings/sinan/apps/desktop/hostservice';
type ViewProps = {plugins: ConfigurablePlugin[]; onChanged():Promise<void>; onRecover():void};
export const pluginViews: Record<string, ComponentType<ViewProps> | undefined> = {
  'system.settings/settings': ({plugins}) => createElement(Settings,{plugins,bridge:settingsBridge}),
  'system.plugin-manager/manager': ({onChanged,onRecover}) => createElement(PluginManager,{bridge:managementBridge,storeBridge,onChanged,onRecover}),
};
// These adapters are private to bundled trusted views, never passed to installed JS.
const managementBridge: ManagementBridge = {
  async trustDevelopment(id,fingerprint){return await TrustDevelopmentPlugin(id,fingerprint);},
  async chooseInstallDirectory(){return await ChooseInstallDirectory();},
  async chooseInstallArchive(){return await ChooseInstallArchive();},
  async chooseDirectory(){return await ChoosePluginDirectory();},
  async createProject(options){return await CreatePluginProject(options);},
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
 windowOpacity:{load:WindowOpacitySettings,save:SaveWindowOpacity,subscribe(changed){return Events.On('window-opacity:changed',event=>changed(event.data as import('../../../../plugins/system-settings/frontend/WindowOpacity').WindowOpacityState));}},
 startup:{load:StartupSettings,silent:SaveSilentStartup,autoStart:SaveAutoStart,subscribe(changed){return Events.On('startup:changed',event=>changed(event.data as import('../../../../plugins/system-settings/frontend/Startup').StartupState));}},
 search:{load:SearchPreferences,save:SaveSearchPreferences,subscribe(changed){return Events.On('search-preferences:changed',event=>changed(event.data as import('../../../../plugins/system-settings/frontend/Search').SearchPreferences));}},
 llm:{async load(){const v=await LLMSettings();return {...v,profiles:v.profiles??[],overrides:Object.fromEntries(Object.entries(v.overrides??{}).map(([k,value])=>[k,value??""]))};},async save(edit){const v=await SaveLLMProfile(edit);return {...v,profiles:v.profiles??[],overrides:Object.fromEntries(Object.entries(v.overrides??{}).map(([k,value])=>[k,value??""]))};},async remove(id,revision){const v=await DeleteLLMProfile(id,revision);return {...v,profiles:v.profiles??[],overrides:Object.fromEntries(Object.entries(v.overrides??{}).map(([k,value])=>[k,value??""]))};},async preferences(p){const v=await SaveLLMPreferences(p);return {...v,profiles:v.profiles??[],overrides:Object.fromEntries(Object.entries(v.overrides??{}).map(([k,value])=>[k,value??""]))};}},
 updates:{load:UpdateStatus,check:CheckHostUpdate,install:InstallHostUpdate,subscribe(changed){return Events.On('host-update:changed',event=>changed(event.data as import('../../../../plugins/system-settings/frontend/Updates').UpdateState));}},
 pluginShortcuts:{
  async list(){return await PluginShortcutSettings()??[];},
  subscribe(changed){return Events.On('plugin-shortcuts:changed',changed);},
  forCommand(pluginID,commandID){return {
   async load(){const rows=await PluginShortcutSettings();const item=rows?.find(row=>row.pluginId===pluginID&&row.commandId===commandID);if(!item)throw new Error('插件快捷键已失效');return item.state;},
   async save(value,revision){return await SavePluginShortcut(pluginID,commandID,value,revision);},
   record(enabled){recordingQueue=recordingQueue.catch(()=>{}).then(()=>RecordShortcut(enabled)).then(()=>{});return recordingQueue;},
   subscribe(changed,captured){const offState=Events.On('plugin-shortcut:changed',event=>{const data=event.data as {pluginId:string;commandId:string;state:ShortcutState};if(data.pluginId===pluginID&&data.commandId===commandID)changed(data.state);});const offCaptured=Events.On('shortcut:recorded',event=>captured(event.data as string));return ()=>{offState();offCaptured();};}
  };}
 },
  theme:{load:ThemeSettings,save:SaveTheme,subscribe(changed){return Events.On("theme:changed",event=>changed(event.data as import("../../../../plugins/system-settings/frontend/Theme").ThemeState));}},
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

const storeBridge:StoreBridge={
 async load(refresh){
  const data = await StoreCatalog(refresh);
  return {...data,entries:(data.entries??[]).map(entry=>({...entry,releases:(entry.releases??[]).map(release=>({...release,assets:release.assets??[]}))}))};
 },
 async install(id,version,revision){return await StoreInstall(id,version,revision);},
 async tasks(){return await StoreInstallTasks()??[];},
 async cancelInstall(id){await StoreCancelInstall(id);},
};
