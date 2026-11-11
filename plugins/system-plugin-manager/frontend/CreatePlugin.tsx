import { useEffect, useRef, useState } from 'react';

export type ProjectOptions = {name: string; id: string; template: string; directory: string};
export type ProjectResult = {id: string; directory: string; packageDirectory: string};
export type ProjectBridge = {
  chooseDirectory(): Promise<string>;
  createProject(options: ProjectOptions): Promise<ProjectResult>;
};

function suggestedID(name: string) {
  const slug = name.normalize('NFKD').toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 96);
  return `local.${slug ? /^[a-z]/.test(slug) ? slug : `plugin-${slug}` : 'my-plugin'}`;
}

export default function CreatePlugin({bridge, disabled, onBusyChange, onClose}: {
  bridge: ProjectBridge; disabled: boolean; onBusyChange(value: boolean): void; onClose(): void;
}) {
  const [name, setName] = useState('');
  const [id, setID] = useState('local.my-plugin');
  const [idEdited, setIDEdited] = useState(false);
  const [template, setTemplate] = useState('go-html');
  const [directory, setDirectory] = useState('');
  const [working, setWorking] = useState(false);
  const [error, setError] = useState('');
  const [result, setResult] = useState<ProjectResult>();
  const panel = useRef<HTMLElement>(null);
  const input = useRef<HTMLInputElement>(null);
  const pending = useRef(false);
  const alive = useRef(true);
  useEffect(() => {
    alive.current = true;
    panel.current?.scrollIntoView({block: 'start'});
    input.current?.focus({preventScroll: true});
    return () => { alive.current = false; };
  }, []);

  async function perform(action: () => Promise<void>) {
    if (pending.current || disabled) return;
    pending.current = true; setWorking(true); onBusyChange(true); setError('');
    try { await action(); }
    catch (reason) { if (alive.current) setError(reason instanceof Error ? reason.message : '操作失败，请重试。'); }
    finally {
      pending.current = false;
      if (alive.current) { setWorking(false); onBusyChange(false); }
    }
  }
  const locked = disabled || working;
  return <section className="create-plugin" ref={panel} aria-labelledby="create-plugin-title">
    <div className="data-toolbar"><h2 id="create-plugin-title">创建插件</h2><button type="button" disabled={locked} onClick={onClose}>{result ? '完成' : '取消'}</button></div>
    {result ? <div className="project-result" role="status"><strong>插件项目已创建</strong><p>{result.directory}</p><small>进入项目先运行 <code>npm ci</code>，再运行 <code>npm run build</code>，再安装 dist/plugin。</small></div> :
      <form onSubmit={event => {event.preventDefault(); void perform(async () => {
        const created = await bridge.createProject({name: name.trim(), id, template, directory});
        if (alive.current) setResult(created);
      });}}>
        <label htmlFor="plugin-name">插件名</label>
        <input ref={input} id="plugin-name" value={name} maxLength={64} required disabled={locked} placeholder="我的工具" onChange={event => {
          setName(event.target.value); if (!idEdited) setID(suggestedID(event.target.value));
        }}/>
        <label htmlFor="plugin-id">插件 ID</label>
        <input id="plugin-id" value={id} maxLength={128} required disabled={locked} pattern={String.raw`[a-z][a-z0-9\-]*(\.[a-z][a-z0-9\-]*)+`} spellCheck={false} autoCapitalize="off" autoCorrect="off" onChange={event => {setID(event.target.value); setIDEdited(true);}}/>
        <label htmlFor="plugin-template">模板</label>
        <select id="plugin-template" value={template} disabled={locked} onChange={event => setTemplate(event.target.value)}>
          <option value="go-html">Go 后端 + React 界面</option><option value="html">React 界面</option>
        </select>
        <label htmlFor="plugin-parent-directory">父目录</label>
        <div className="install-row"><input id="plugin-parent-directory" value={directory} readOnly placeholder="选择项目保存位置" title={directory}/><button type="button" disabled={locked} onClick={() => void perform(async () => {
          const selected = await bridge.chooseDirectory();
          if (selected && alive.current) setDirectory(selected);
        })}>选择目录</button></div>
        {directory && <p className="project-path">将创建：{directory.replace(/[\\/]$/, '')}/{id}</p>}
        <button className="primary" disabled={locked || !name.trim() || !directory}>{working ? '处理中…' : '创建项目'}</button>
      </form>}
    {error && <p className="error" role="alert">{error}</p>}
  </section>;
}
