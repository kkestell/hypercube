// A unified whole-file diff with separately highlighted old and new text.
import {Table, TableBody, TableCell, TableRow} from '../components/ui/table';
import {Badge} from '../components/ui/badge';
import {useEffect, useMemo, useRef, useState} from 'react';
import {useScrollMemory} from '../editor/use-scroll-memory.js';
import {language} from '../files/language.js';
import {useStore} from '../store.js';
import {diffNote, diffRows} from './diff.js';

function DiffLines({text, path}) {
  const rows = useMemo(() => diffRows(text), [text]);
  const [highlighted, setHighlighted] = useState(null);
  useEffect(() => {
    if (!rows.length) return;
    let current = true;
    import('../files/highlight.js').then(({highlight}) => {
      if (!current) return;
      const lang = language(path.slice(path.lastIndexOf('/') + 1));
      const old = highlight(rows.filter(row => row.kind !== 'added').map(row => row.text).join('\n'), lang);
      const next = highlight(rows.filter(row => row.kind !== 'removed').map(row => row.text).join('\n'), lang);
      setHighlighted({text, old, next});
    });
    return () => { current = false; };
  }, [rows, path]);
  if (!rows.length) return <div className="diff-view-message p-6 text-center text-muted-foreground">{diffNote(text)}</div>;
  const html = highlighted?.text === text ? highlighted : null;
  return <Table className="diff-view-lines font-mono text-xs"><TableBody>{rows.map((row, i) => {
    const line = row.kind === 'removed' ? html?.old[row.old - 1] : html?.next[row.new - 1];
    return (
      <TableRow key={i} className={`diff-view-line diff-view-${row.kind}`}>
        <TableCell className="w-12 text-right text-muted-foreground">{row.old}</TableCell><TableCell className="w-12 text-right text-muted-foreground">{row.new}</TableCell><TableCell className="w-8">{row.kind !== 'context' && <Badge variant={row.kind === 'removed' ? 'destructive' : 'secondary'}>{row.kind === 'added' ? '+' : '−'}</Badge>}</TableCell>
        <TableCell className="whitespace-pre">{line === undefined ? <code>{row.text}</code> : <code dangerouslySetInnerHTML={{__html: line}} />}</TableCell>
      </TableRow>
    );
  })}</TableBody></Table>;
}

export function DiffView({id, path}) {
  const [state] = useStore();
  const view = useRef(null);
  useScrollMemory(view, id);
  const diff = state.session.diffs[path];
  return (
    <div ref={view} className="diff-view min-h-0 flex-1 overflow-auto">
      {diff && (diff.message !== null ? <div className="diff-view-message p-6 text-center text-muted-foreground">{diff.message}</div>
        : diff.text !== null ? <DiffLines text={diff.text} path={path} /> : null)}
    </div>
  );
}
