/** v1 wire contract; this module does not expose the Wails bridge. */
export const API_VERSION = '1' as const;
export interface InitializeParams { hostCapabilities?: string[]; settings?: Record<string,string[]>; apiVersion: string; pluginId: string; cacheDir?: string; dataDir?: string }
export interface InitializeResult { apiVersion: string; pluginId: string; capabilities?: string[] }
export interface CommandParams { commandId: string; input: string }
export interface CommandResult { text: string; dismiss?: boolean; keepHidden?: boolean }
export interface Request<T> { jsonrpc: '2.0'; id?: number; method: string; params?: T }
export type Response<T> = { jsonrpc: '2.0'; id: number } & (
  | { result: T; error?: never }
  | { error: { code: number; message: string }; result?: never }
);

export interface CancelParams { id: number }
export interface SearchParams { providerId: string; query: string; limit: number }
export interface SearchItem { icon?: string; id: string; title: string; subtitle?: string; commandId: string; input?: string; score: number }
export interface SearchResult { items: SearchItem[] }

export interface EntryRequirement {id:string;version:"1"}
export interface PluginDependency {id:string;pluginId:string;minVersion:string;optional:boolean;entries:EntryRequirement[]}
export interface EntryPoint {id:string;version:"1";commandId:string;input:"image"}
export interface UIView { dependencies?:string[]; id:string; entry:string; title?:string; bridgeVersion:'1'|'2'; commands?:string[]; permissions?:('storage'|'input.paste'|'storage.encrypted'|'llm'|'clipboard.read'|'clipboard.write'|'window.execute'|'window.show'|'window.hide'|'file.savePNG'|'file.openImage')[] }
export type UIRequest = {method:'dependency.status'|'dependency.manage'} | {method:'dependency.open'|'image.upload.start';input:string;value:unknown} | {method:'image.upload.chunk'|'image.upload.finish'|'image.upload.cancel';input:string} | {method:'clipboard.readText'} | {method:'clipboard.writeText';input:string} | {method:'llm.config'} | {method:'llm.start';value:{profileId?:string;systemPrompt:string;input:string}} | {method:'llm.poll'|'llm.cancel';input:string} | {method:'command.execute'|'window.execute'|'file.savePNG';commandId:string;input:string} | {method:'input.paste'|'window.show'|'window.hide'|'file.openImage'} | {method:'launch.get'} | {method:'storage.get'} | {method:'storage.encrypted.get'} | {method:'storage.encrypted.put';revision:number;value:unknown} | {method:'storage.put';revision:number;value:unknown};
export interface PluginDocument<T=unknown> {revision:number;value:T|null}

/** Data-only contract reserved for future theme providers; no CSS or scripts. */
export interface ThemeDefinition {version:'1';id:string;name:string;appearance:'light'|'dark';tokens:Record<string,string>}
export interface ThemeState {mode:'system'|'light'|'dark';revision:number;light:ThemeDefinition;dark:ThemeDefinition;error?:string}

export const CLIPBOARD_CAPABILITY = 'clipboard.v1' as const;
/** data is base64, image content is PNG. */
export interface ClipboardContent {kind:'text'|'image'|'';data:string|null}
export interface ClipboardParams {sequence?:number;content?:ClipboardContent}
export interface ClipboardResult {sequence:number;content:ClipboardContent}
