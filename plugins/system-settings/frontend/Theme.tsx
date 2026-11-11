import { useEffect, useRef, useState } from 'react';

export type ThemeState = { mode: string; revision: number; error?: string };
export type ThemeBridge = {
  load(): Promise<ThemeState>;
  save(mode: string, revision: number): Promise<ThemeState>;
  subscribe(changed: (state: ThemeState) => void): () => void;
};

export default function Theme({ bridge }: { bridge: ThemeBridge }) {
  const [state, setState] = useState<ThemeState>();
  const [pending, setPending] = useState(false);
  const [error, setError] = useState('');
  const revision = useRef(-1), alive = useRef(false), saving = useRef(false);

  function receive(next: ThemeState) {
    if (next.revision < revision.current) return;
    revision.current = next.revision;
    setState(next);
  }

  useEffect(() => {
    alive.current = true;
    const off = bridge.subscribe(receive);
    void bridge.load().then(next => {
      if (alive.current) receive(next);
    }).catch(() => {
      if (alive.current) setError('无法读取主题设置。');
    });
    return () => { alive.current = false; off(); };
  }, [bridge]);

  async function save(mode: string) {
    if (!state || saving.current || mode === state.mode) return;
    saving.current = true;
    setPending(true);
    setError('');
    try {
      const next = await bridge.save(mode, state.revision);
      if (alive.current) receive(next);
    } catch (reason) {
      if (alive.current) {
        setError(reason instanceof Error ? reason.message : '主题保存失败。');
        try {
          const latest = await bridge.load();
          if (alive.current) receive(latest);
        } catch { /* 保留保存失败提示。 */ }
      }
    } finally {
      saving.current = false;
      if (alive.current) setPending(false);
    }
  }

  return <section className="settings-theme" aria-label="外观">
    <h2>外观</h2>
    <div className="theme-options">
      {[['light', '浅色'], ['dark', '深色'], ['system', '跟随系统']].map(([mode, label]) =>
        <button key={mode} type="button" aria-pressed={state?.mode === mode}
          disabled={!state || pending || !!state.error} onClick={() => void save(mode)}>{label}</button>)}
    </div>
    {(error || state?.error) && <p className="error" role="alert">{error || state?.error}</p>}
  </section>;
}
