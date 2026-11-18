import { useEffect, useState } from 'react';

export type LauncherPositionState = {
  topOffsetPercent: number;
  revision: number;
  error?: string;
};

export type LauncherPositionBridge = {
  load(): Promise<LauncherPositionState>;
  save(percent: number, revision: number): Promise<LauncherPositionState>;
  subscribe(changed: (state: LauncherPositionState) => void): () => void;
};

export default function LauncherPosition({ bridge }: { bridge: LauncherPositionBridge }) {
  const [state, setState] = useState<LauncherPositionState>();
  const [draft, setDraft] = useState<string>();
  const [pending, setPending] = useState(false);
  const [error, setError] = useState('');
  const value = draft ?? String(state?.topOffsetPercent ?? 20);
  const percent = Number(value);
  const disabled = !state || pending || !!state.error;

  function receive(next: LauncherPositionState) {
    setState(current => current && current.revision > next.revision ? current : next);
  }

  useEffect(() => {
    let active = true;
    const off = bridge.subscribe(next => { if (active) receive(next); });
    void bridge.load().then(next => { if (active) receive(next); })
      .catch(() => { if (active) setError('无法读取搜索框位置。'); });
    return () => { active = false; off(); };
  }, [bridge]);

  async function save() {
    if (!state || disabled || value === '' || !Number.isInteger(percent) || percent < 0 || percent > 50 || percent === state.topOffsetPercent) return;
    setPending(true);
    setError('');
    try {
      receive(await bridge.save(percent, state.revision));
      setDraft(undefined);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '搜索框位置保存失败。');
      try { receive(await bridge.load()); } catch { /* 保留保存错误。 */ }
    } finally {
      setPending(false);
    }
  }

  return <form className="settings-section" onSubmit={event => { event.preventDefault(); void save(); }} aria-label="搜索框位置">
    <div className="settings-number-row">
      <label htmlFor="launcher-top-offset">搜索框顶边距</label>
      <div>
        <input id="launcher-top-offset" type="number" min="0" max="50" step="1" required value={value}
          disabled={disabled} aria-describedby="launcher-top-offset-note"
          onChange={event => { setDraft(event.target.value); setError(''); }} />
        <span aria-hidden="true">%</span>
      </div>
    </div>
    <div className="settings-save">
      <span className="settings-note" id="launcher-top-offset-note">占屏幕可用高度的百分比，下次显示生效</span>
      <button className="primary" disabled={disabled || value === '' || percent === state?.topOffsetPercent}>{pending ? '保存中…' : '保存'}</button>
    </div>
    {(error || state?.error) && <p className="error" role="alert">{error || state?.error}</p>}
  </form>;
}
