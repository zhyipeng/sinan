import React, { useEffect, useRef, useState } from 'react';
import { readData, writeData } from './bridge.js';

export default function App() {
  const [text, setText] = useState('');
  const [output, setOutput] = useState('');
  const [status, setStatus] = useState('正在读取草稿…');
  const [available, setAvailable] = useState(false);
  const [working, setWorking] = useState(false);
  const revision = useRef(0);
  const pending = useRef(false);

  useEffect(() => {
    let active = true;
    readData().then(draft => {
      if (!active) return;
      revision.current = draft.revision;
      setText(typeof draft.value?.text === 'string' ? draft.value.text : '');
      setAvailable(true);
      setStatus('');
    }).catch(() => { if (active) setStatus('无法读取草稿，请返回后重新打开。'); });
    return () => { active = false; };
  }, []);

  async function submit(event) {
    event.preventDefault();
    if (!available || pending.current) return;
    pending.current = true;
    setWorking(true);
    try {
      setOutput(text);
      const saved = await writeData(revision.current, { text });
      revision.current = saved.revision;
      setStatus('已保存');
    } catch { setStatus('保存失败，请返回后重新打开；当前输入已保留。'); }
    finally { pending.current = false; setWorking(false); }
  }

  return <main className="plugin-body">
    <form onSubmit={submit}>
      <label htmlFor="input">输入内容</label>
      <textarea id="input" value={text} onChange={event => setText(event.target.value)} disabled={!available || working} placeholder="输入内容…" />
      <div className="form-actions"><p role="status">{status}</p><button className="primary" disabled={!available || working}>{working ? '处理中…' : '保存'}</button></div>
    </form>
    {output && <output aria-live="polite">{output}</output>}
  </main>;
}
