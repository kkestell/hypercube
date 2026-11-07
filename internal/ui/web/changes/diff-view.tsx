// A unified whole-file diff with separately highlighted old and new text.
import {useEffect, useMemo, useRef} from 'react';
import {matches} from '../app/shortcuts.ts';
import {Button} from '../components/ui/button';
import {useScrollMemory} from '../editor/use-scroll-memory.ts';
import type {TabId} from '../editor/types.ts';
import {language} from '../files/language.ts';
import {useHighlight} from '../files/use-highlight.ts';
import {Find} from '../shared/find.tsx';
import {Icon} from '../shared/icon.tsx';
import {useStore} from '../store.ts';
import {diffReady, readOpenDiff} from './changes.ts';
import {diffNote, diffRows} from './diff.ts';
import type {DiffRow} from './types.ts';

const background: Record<DiffRow['kind'], string> = {context: '', added: 'bg-green-500/15', removed: 'bg-red-500/15'};

function DiffLines({text, path, wrap, active}: {text: string; path: string; wrap: boolean; active: boolean}) {
  const rows = useMemo(() => diffRows(text), [text]);
  const [old, next] = useMemo(() => [
    rows.filter(row => row.kind !== 'added').map(row => row.text).join('\n'),
    rows.filter(row => row.kind !== 'removed').map(row => row.text).join('\n'),
  ], [rows]);
  const html = useHighlight([old, next], language(path.slice(path.lastIndexOf('/') + 1)), active && rows.length > 0);
  if (!rows.length) return <div className="diff-view-message p-6 text-center text-muted-foreground">{diffNote(text)}</div>;
  return <table className={`diff-view-lines w-full border-collapse font-mono text-xs leading-5 ${wrap ? "table-fixed" : ""}`}><tbody>{rows.map((row, i) => {
    const line = row.kind === 'removed' ? html?.[0]?.[row.old - 1] : html?.[1]?.[row.new - 1];
    return (
      <tr key={i} data-change={row.kind !== 'context' && (i === 0 || rows[i - 1].kind === 'context') ? '' : undefined} className={`diff-view-line diff-view-${row.kind} ${background[row.kind]}`}>
        <td data-find-ignore className="w-12 px-2 py-0 text-right align-top text-muted-foreground">{row.old}</td><td data-find-ignore className="w-12 px-2 py-0 text-right align-top text-muted-foreground">{row.new}</td><td data-find-ignore className="w-6 px-1 py-0 text-center align-top text-muted-foreground">{row.kind === 'added' ? '+' : row.kind === 'removed' ? '−' : ''}</td>
        <td className={`${wrap ? "whitespace-pre-wrap break-all" : "whitespace-pre"} px-2 py-0 align-top`}>{line === undefined ? <code>{row.text}</code> : <code dangerouslySetInnerHTML={{__html: line}} />}</td>
      </tr>
    );
  })}</tbody></table>;
}

export function DiffView({id, path, active = true}: {id: TabId; path: string; active?: boolean}) {
  const [state, store] = useStore();
  const view = useRef<HTMLDivElement>(null), list = useRef<HTMLDivElement>(null);
  const diff = state.session?.diffs[path];
  const sessionId = state.session?.id;
  const ready = diffReady(diff);
  useScrollMemory(view, id, ready);
  useEffect(() => {
    if (sessionId && diff && !ready) readOpenDiff(store, path);
  }, [sessionId, path, !!diff, ready]);
  const move = (direction: number) => {
    if (!list.current || !view.current) return;
    const changes = [...list.current.querySelectorAll('[data-change]')];
    if (!changes.length) return;
    const top = view.current.getBoundingClientRect().top;
    const target = direction > 0 ? changes.find(row => row.getBoundingClientRect().top > top)
      : changes.findLast(row => row.getBoundingClientRect().top < top);
    (target ?? changes[direction > 0 ? 0 : changes.length - 1]).scrollIntoView({block: 'start'});
  };
  useEffect(() => {
    const keydown = (event: KeyboardEvent) => {
      const pane = view.current?.closest('.editor-pane');
      if (!pane?.contains(document.activeElement) || !(matches(event, 'nextChange') || matches(event, 'previousChange'))) return;
      event.preventDefault();
      move(matches(event, 'previousChange') ? -1 : 1);
    };
    document.addEventListener('keydown', keydown);
    return () => document.removeEventListener('keydown', keydown);
  }, [diff?.text]);
  const hasChanges = diff?.text && diffRows(diff.text).some(row => row.kind !== 'context');
  return (
    <div className="relative flex min-h-0 flex-1 flex-col">
      <div className="flex justify-end border-b p-1">
        <Button variant="ghost" size="icon" aria-label="Previous change" title="Previous change (Shift+Option+F5)" disabled={!hasChanges} onClick={() => move(-1)}><Icon name="arrow-up" /></Button>
        <Button variant="ghost" size="icon" aria-label="Next change" title="Next change (Option+F5)" disabled={!hasChanges} onClick={() => move(1)}><Icon name="arrow-down" /></Button>
      </div>
      <Find list={list} view={view} active={active} label="Find in diff" className="absolute right-2 top-2 z-10 flex items-center gap-2 rounded-md border bg-background p-2 shadow-md" />
      <div ref={view} tabIndex={0} className="diff-view min-h-0 flex-1 overflow-auto">
        <div ref={list}>{diff && (diff.message !== null ? <div className="diff-view-message p-6 text-center text-muted-foreground">{diff.message}</div>
          : diff.text !== null ? <DiffLines text={diff.text} path={path} wrap={state.workbench.wrap} active={active} /> : null)}</div>
      </div>
    </div>
  );
}
