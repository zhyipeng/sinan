/** v1 wire contract; this module does not expose the Wails bridge. */
export const API_VERSION = '1' as const;
export interface InitializeParams { apiVersion: string; pluginId: string }
export interface InitializeResult { apiVersion: string; pluginId: string }
export interface CommandParams { commandId: string; input: string }
export interface CommandResult { text: string }
export interface Request<T> { jsonrpc: '2.0'; id: number; method: string; params?: T }
export type Response<T> = { jsonrpc: '2.0'; id: number } & (
  | { result: T; error?: never }
  | { error: { code: number; message: string }; result?: never }
);
