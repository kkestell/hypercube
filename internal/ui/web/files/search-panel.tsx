import {useLayoutEffect, useRef, useState} from 'react';
import {searchFiles} from '../api.ts';
import {Button} from '../components/ui/button';
import {Input} from '../components/ui/input';
import {openFileAt} from '../editor/editor.ts';
import {useStore} from '../store.ts';
import {Message} from '../shared/message.tsx';
import {useRequest} from '../shared/use-request.ts';
import {fileRow} from './file-row.ts';

export function SearchPanel() {
  const [state, store] = useStore();
  const [query, setQuery] = useState('');
  const input = useRef<HTMLInputElement>(null);
  const editor = state.editor?.id;
  useLayoutEffect(() => {
    if (state.workbench.rightPanel && state.workbench.rightTab === 'search') input.current?.focus();
  }, [state.workbench.rightPanel, state.workbench.rightTab]);
  const {data: results = [], error, loading} = useRequest(editor && query ? signal => searchFiles(state.editor!.target, query, signal) : null, [editor, query]);
  return <div className="p-2 pt-1">
    <Input ref={input} aria-label="Search files contents" placeholder="Search" value={query} onChange={e => setQuery(e.target.value)} />
    {error && <Message error className="p-2">Could not search files: {error}</Message>}
    {loading && <Message className="p-2">Searching files.</Message>}
    {editor && results.map(file => <section key={file.path} className="mt-3">
      <h3 className="truncate px-2 pb-0.5 text-sm font-semibold" title={file.path}>{file.path}</h3>
      {file.lines.map(line => <Button key={line.line} variant="ghost" size="sm" className={`${fileRow} overflow-hidden text-left font-mono text-xs`}
        title={line.parts.join('')}
        onClick={() => openFileAt(store, file.path, [{start: line.line, end: line.line}])}>
        <span className="w-10 shrink-0 text-right text-muted-foreground tabular-nums">{line.line}</span><span className="truncate">{line.parts.map((part, i) => i % 2 ? <mark key={i}>{part}</mark> : part)}</span>
      </Button>)}
    </section>)}
  </div>;
}
