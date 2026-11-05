import { useEffect, useRef, useState, type KeyboardEvent } from 'react';

export type ShortcutState = { shortcut: string; active: string; default: string; revision: number; error: string; ready: boolean };
export type ShortcutBridge = {
  load(): Promise<ShortcutState>;
  save(value: string, revision: number): Promise<ShortcutState>;
  record(enabled: boolean): Promise<void>;
  subscribe(changed: (state: ShortcutState) => void, captured: (key: string) => void): () => void;
};
function format(value: string) {
  return value.split('+').map(key => ({ Cmd: 'Command', Alt: /mac/i.test(navigator.platform) ? 'Option' : 'Alt', Super: 'Super', Space: 'Space' }[key] ?? key)).join(' + ');
}
function keyCombination(event: KeyboardEvent): string | undefined {
  const code = event.code;
  let key = /^Key[A-Z]$/.test(code) ? code.slice(3) : /^Digit[0-9]$/.test(code) ? code.slice(5) : /^F([1-9]|1[0-9]|20)$/.test(code) ? code : undefined;
  key ??= ({ Space: 'Space', Enter: 'Enter', Tab: 'Tab', Backspace: 'Backspace', Delete: 'Delete', Home: 'Home', End: 'End', PageUp: 'Page Up', PageDown: 'Page Down', ArrowUp: 'Up', ArrowDown: 'Down', ArrowLeft: 'Left', ArrowRight: 'Right' } as Record<string, string>)[code];
  if (!key) return;
  if (!event.metaKey && !event.ctrlKey && !event.altKey) return;
  const modifiers = [event.ctrlKey && 'Ctrl', event.altKey && 'Alt', event.shiftKey && 'Shift', event.metaKey && (/mac/i.test(navigator.platform) ? 'Cmd' : 'Super')].filter(Boolean);
  return [...modifiers, key].join('+');
}
export default function Shortcut({ bridge }: { bridge: ShortcutBridge }) {
  const [state, setState] = useState<ShortcutState>();
  const [draft, setDraft] = useState('');
  const [recording, setRecording] = useState(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const recorder = useRef<HTMLButtonElement>(null);
  const alive = useRef(false);
  const recordingRef = useRef(false);
  const dirty = useRef(false);
  function stop() {
    recordingRef.current = false; setRecording(false);
    void bridge.record(false).catch(() => {});
  }
  function capture(value: string) {
    if (!recordingRef.current) return;
    dirty.current = true; setDraft(value); setError(''); setNotice(''); stop();
  }
  useEffect(() => {
    alive.current = true;
    const apply = (next: ShortcutState) => { if (alive.current) { setState(next); if (!dirty.current) setDraft(next.shortcut); } };
    const unsubscribe = bridge.subscribe(apply, capture);
    bridge.load().then(apply).catch(() => { if (alive.current) setError('无法读取快捷键，请重新打开设置。'); });
    return () => { alive.current = false; recordingRef.current = false; unsubscribe(); void bridge.record(false).catch(() => {}); };
  }, [bridge]);
  useEffect(() => {
    if (!recording) return;
    recorder.current?.focus();
    const timeout = window.setTimeout(stop, 30000);
    return () => window.clearTimeout(timeout);
  }, [recording]);
  async function start() {
    if (pending || recordingRef.current || !state?.ready) return;
    setPending(true); setError(''); setNotice('');
    try {
      await bridge.record(true);
      if (!alive.current) return;
      recordingRef.current = true; setRecording(true);
    } catch { if (alive.current) setError('无法开始录入，请重试。'); }
    finally { if (alive.current) setPending(false); }
  }
  async function save() {
    if (!state || pending || recording) return;
    setPending(true); setError(''); setNotice('');
    try {
      const next = await bridge.save(draft, state.revision);
      if (alive.current) { dirty.current = false; setState(next); setDraft(next.shortcut); setNotice('快捷键已保存并生效。'); }
    } catch (error) { if (alive.current) setError(error instanceof Error ? error.message : '快捷键保存失败，请重试。'); }
    finally { if (alive.current) setPending(false); }
  }
  return <section className="settings-shortcut">
    <h2>呼出快捷键</h2>
    <div className="shortcut-controls">
      <button ref={recorder} type="button" className={`shortcut-recorder ${recording ? 'recording' : ''}`} aria-label="录入呼出快捷键" aria-pressed={recording} disabled={pending || !state?.ready} onClick={() => void start()} onBlur={() => { if (recordingRef.current) stop(); }} onKeyDown={event => {
        if (!recordingRef.current || event.nativeEvent.isComposing) return;
        if (event.key === 'Tab' && !event.metaKey && !event.ctrlKey && !event.altKey) { stop(); return; }
        event.preventDefault(); event.stopPropagation();
        if (event.key === 'Escape') { stop(); return; }
        if (['Meta', 'Control', 'Alt', 'Shift'].includes(event.key)) return;
        const value = keyCombination(event);
        if (value) capture(value); else setError('请同时按下 Command、Ctrl 或 Alt 与一个标准按键。');
      }}>{recording ? '请按组合键…' : draft ? format(draft) : '正在读取…'}</button>
      <button type="button" disabled={pending || !state} onClick={() => { stop(); dirty.current = true; setDraft(state!.default); setError(''); setNotice(''); }}>恢复默认</button>
      <button type="button" className="primary" disabled={pending || recording || !state?.ready || draft === state.shortcut && draft === state.active && !state.error} onClick={() => void save()}>{pending ? '处理中…' : '保存快捷键'}</button>
    </div>
    <p className="settings-note">点击左侧后按下组合键，保存后立即生效</p>
    {recording && <button type="button" className="shortcut-cancel" onClick={stop}>取消录入</button>}
    {(error || state?.error) && <p className="error" role="alert">{error || state?.error}</p>}
    {notice && <p className="settings-notice" role="status">{notice}</p>}
  </section>;
}
