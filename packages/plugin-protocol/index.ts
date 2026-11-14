/** v1 wire contract; this module does not expose the Wails bridge. */
export const API_VERSION = '1' as const;
export interface InitializeParams { settings?: Record<string,string[]>; apiVersion: string; pluginId: string; cacheDir?: string; dataDir?: string }
export interface InitializeResult { apiVersion: string; pluginId: string; capabilities?: string[] }
export interface CommandParams { commandId: string; input: string }
export interface CommandResult { text: string; dismiss?: boolean }
export interface Request<T> { jsonrpc: '2.0'; id?: number; method: string; params?: T }
export type Response<T> = { jsonrpc: '2.0'; id: number } & (
  | { result: T; error?: never }
  | { error: { code: number; message: string }; result?: never }
);

export interface CancelParams { id: number }
export interface SearchParams { providerId: string; query: string; limit: number }
export interface SearchItem { icon?: string; id: string; title: string; subtitle?: string; commandId: string; input?: string; score: number }
export interface SearchResult { items: SearchItem[] }

export interface UIView { id:string; entry:string; title?:string; bridgeVersion:'1'|'2'; commands?:string[]; permissions?:('storage'|'input.paste')[] }
export type UIRequest = {method:'command.execute';commandId:string;input:string} | {method:'input.paste'} | {method:'launch.get'} | {method:'storage.get'} | {method:'storage.put';revision:number;value:unknown};
export interface PluginDocument<T=unknown> {revision:number;value:T|null}

/** Data-only contract reserved for future theme providers; no CSS or scripts. */
export interface ThemeDefinition {version:'1';id:string;name:string;appearance:'light'|'dark';tokens:Record<string,string>}
export interface ThemeState {mode:'system'|'light'|'dark';revision:number;light:ThemeDefinition;dark:ThemeDefinition;error?:string}
