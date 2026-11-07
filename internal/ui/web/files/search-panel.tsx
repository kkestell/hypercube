import {useEffect, useLayoutEffect, useRef, useState} from 'react';
import {searchFiles} from '../api.ts';
import {Button} from '../components/ui/button';
import {Input} from '../components/ui/input';
import {openRequestedFile} from '../editor/editor.ts';
import {useStore} from '../store.ts';
import type {SearchFile} from './types.ts';

export function SearchPanel() {
  const [state, store] = useStore();
  const [query, setQuery] = useState('');
  const [results, setResults] = useState<SearchFile[]>([]);
  const [error, setError] = useState('');
  const input = useRef<HTMLInputElement>(null);
  const session = state.session?.id;
  useLayoutEffect(() => {
    if (state.workbench.rightPanel && state.workbench.rightTab === 'search') input.current?.focus();
  }, [state.workbench.rightPanel, state.workbench.rightTab]);
  useEffect(() => {
    if (!session || !query) { setResults([]); setError(''); return; }
    const controller = new AbortController();
    setResults([]); setError('');
    searchFiles(session, query, controller.signal)
      .then(data => { if (!controller.signal.aborted) setResults(data); })
      .catch((err: unknown) => { if (!controller.signal.aborted) setError('Could not search files: ' + (err instanceof Error ? err.message : String(err))); });
    return () => controller.abort();
  }, [session, query]);
  return <div className="p-2 pt-1">
    <Input ref={input} aria-label="Search files contents" placeholder="Search" value={query} onChange={e => setQuery(e.target.value)} />
    {error && <p className="p-2 text-sm">{error}</p>}
    {session && results.map(file => <section key={file.path} className="mt-3">
      <h3 className="truncate px-2 pb-0.5 text-xs leading-4 font-medium" title={file.path}>{file.path}</h3>
      {file.lines.map(line => <Button key={line.line} variant="ghost" className="h-auto w-full justify-start overflow-hidden rounded-sm px-2 py-0.5 text-left font-mono text-xs leading-4 font-normal"
        title={line.parts.join('')}
        onClick={() => openRequestedFile(store, {session, path: file.path, lines: [{start: line.line, end: line.line}]})}>
        <span className="w-10 shrink-0 text-right text-muted-foreground tabular-nums">{line.line}</span><span className="truncate">{line.parts.map((part, i) => i % 2 ? <mark key={i}>{part}</mark> : part)}</span>
      </Button>)}
    </section>)}
  </div>;
}
