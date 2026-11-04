// A unified whole-file diff with separately highlighted old and new text.
import {useEffect, useMemo, useRef, useState} from 'preact/hooks';
import {useScrollMemory} from '../editor/use-scroll-memory.js';
import {language} from '../files/language.js';
import {useStore} from '../store.js';
import {diffRows} from './diff.js';
import './diff-view.css';

function DiffLines({text, path}) {
  const rows = useMemo(() => diffRows(text), [text]);
  const [highlighted, setHighlighted] = useState(null);
  useEffect(() => {
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
  if (!rows.length) return <div class="diff-view-message">No changes</div>;
  const html = highlighted?.text === text ? highlighted : null;
  return <div class="diff-view-lines">{rows.map((row, i) => {
    const line = row.kind === 'removed' ? html?.old[row.old - 1] : html?.next[row.new - 1];
    return (
      <div key={i} class={`diff-view-line diff-view-${row.kind}`}>
        <span>{row.old}</span><span>{row.new}</span><span>{row.kind === 'added' ? '+' : row.kind === 'removed' ? '-' : ''}</span>
        {line === undefined ? <code>{row.text}</code> : <code dangerouslySetInnerHTML={{__html: line}} />}
      </div>
    );
  })}</div>;
}

export function DiffView({id, path}) {
  const [state] = useStore();
  const view = useRef(null);
  useScrollMemory(view, id);
  const diff = state.session.diffs[path];
  return (
    <div ref={view} class="diff-view">
      {diff && (diff.message !== null ? <div class="diff-view-message">{diff.message}</div>
        : diff.text !== null ? <DiffLines text={diff.text} path={path} /> : null)}
    </div>
  );
}
