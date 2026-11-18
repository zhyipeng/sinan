import { useEffect, useState } from 'react';

export type UpdateState = {
  currentVersion: string;
  latestVersion: string;
  phase: string;
  message: string;
  available: boolean;
  supported: boolean;
  installationResult: string;
  downloadedBytes: number;
  totalBytes: number;
};

export type UpdateBridge = {
  load(): Promise<UpdateState>;
  check(): Promise<UpdateState>;
  install(): Promise<void>;
  subscribe(changed: () => void): () => void;
};

function useUpdateState(bridge: UpdateBridge, trackProgress = false) {
  const [state, setState] = useState<UpdateState>();
  useEffect(() => {
    let active = true;
    let sequence = 0;
    let downloading = false;
    let timer: ReturnType<typeof setTimeout> | undefined;

    function refresh() {
      clearTimeout(timer);
      const request = ++sequence;
      void bridge.load().then(next => {
        if (!active || request !== sequence) return;
        downloading = next.phase === 'downloading';
        setState(next);
      }).catch(() => {}).finally(() => {
        if (active && request === sequence && trackProgress && downloading && !document.hidden) {
          timer = setTimeout(refresh, 500);
        }
      });
    }

    function onVisibilityChange() {
      if (document.hidden) clearTimeout(timer);
      else refresh();
    }

    const unsubscribe = bridge.subscribe(refresh);
    document.addEventListener('visibilitychange', onVisibilityChange);
    refresh();
    return () => {
      active = false;
      clearTimeout(timer);
      unsubscribe();
      document.removeEventListener('visibilitychange', onVisibilityChange);
    };
  }, [bridge, trackProgress]);
  return state;
}

const phaseMessages: Record<string, string | undefined> = {
  checking: '正在检查更新…',
  downloading: '正在下载更新…',
  preparing: '正在准备安装…',
  installing: '正在退出并安装…',
};

export function UpdateNotice({ bridge, onOpen }: { bridge: UpdateBridge; onOpen: () => void }) {
  const state = useUpdateState(bridge);
  if (!state?.available) return null;
  const updating = state.phase === 'downloading' || state.phase === 'preparing' || state.phase === 'installing';
  return <button className="update-notice" title={`司南 ${state.latestVersion} 可用`} onClick={onOpen}>
    {updating ? '更新中' : '有更新'}
  </button>;
}

export default function Updates({ bridge }: { bridge: UpdateBridge }) {
  const state = useUpdateState(bridge, true);
  const [error, setError] = useState('');
  const [pending, setPending] = useState(false);
  const phaseMessage = state && phaseMessages[state.phase];
  const busy = pending || !!phaseMessage;
  const message = phaseMessage || state?.message || (state?.available ? `新版本 ${state.latestVersion} 可用` : '启动后自动检查稳定版本');
  const progress = state?.phase === 'downloading' && state.totalBytes > 0 ? state : undefined;

  async function run(action: () => Promise<unknown>, failure: string) {
    setPending(true);
    setError('');
    try {
      await action();
    } catch {
      setError(failure);
    } finally {
      setPending(false);
    }
  }

  return <section className="settings-section" aria-label="司南更新">
    <h2>司南更新{state?.currentVersion && <small className="settings-note"> · {state.currentVersion}</small>}</h2>
    <p className="settings-note" role="status">{message}</p>
    {progress && <div className="settings-update-progress">
      <progress aria-label="司南更新下载进度" max={progress.totalBytes} value={progress.downloadedBytes} />
      <span aria-hidden="true">{Math.floor(progress.downloadedBytes / progress.totalBytes * 100)}%</span>
    </div>}
    {state?.installationResult && <p className="settings-note">{state.installationResult}</p>}
    <div className="settings-save">
      <button disabled={!state?.supported || busy} onClick={() => void run(bridge.check, '检查更新失败，请稍后重试。')}>检查更新</button>
      {state?.available && <button className="primary" disabled={busy} onClick={() => void run(bridge.install, '无法开始更新，请检查状态后重试。')}>下载并重启安装</button>}
    </div>
    {error && <p className="error" role="alert">{error}</p>}
  </section>;
}
