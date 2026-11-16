import { useEffect, useRef, useState } from 'react';

export type SearchPreferences = { spaceToOpen: boolean; revision: number; error?: string };
export type SearchBridge = {
  load(): Promise<SearchPreferences>;
  save(spaceToOpen: boolean, revision: number): Promise<SearchPreferences>;
  subscribe(changed: (state: SearchPreferences) => void): () => void;
};

export default function Search({ bridge }: { bridge: SearchBridge }) {
  const [state, setState] = useState<SearchPreferences>();
  const [pending, setPending] = useState(false);
  const [error, setError] = useState('');
  const revision = useRef(-1), alive = useRef(false), saving = useRef(false);

  function receive(next: SearchPreferences) {
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
      if (alive.current) setError('无法读取搜索设置。');
    });
    return () => { alive.current = false; off(); };
  }, [bridge]);

  async function save(spaceToOpen: boolean) {
    if (!state || saving.current || spaceToOpen === state.spaceToOpen) return;
    saving.current = true;
    setPending(true);
    setError('');
    try {
      const next = await bridge.save(spaceToOpen, state.revision);
      if (alive.current) receive(next);
    } catch (reason) {
      if (alive.current) {
        setError(reason instanceof Error ? reason.message : '搜索保存失败。');
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

  return <section className="settings-section" aria-label="搜索">
    <label className="search-open-toggle">
      <span>空格键打开</span>
      <input type="checkbox" role="switch" checked={state?.spaceToOpen ?? false}
        disabled={!state || pending || !!state.error} onChange={event => void save(event.target.checked)} />
    </label>
    {(error || state?.error) && <p className="error" role="alert">{error || state?.error}</p>}
  </section>;
}
