// A workspace's sessions as a keyboard-navigable tree, with each parent's visible
// children following it. An active parent lists its running children and folds
// its ended ones behind one row, which keeps the shown session in view. One row
// is tabbable: the focused row, else the selected one, else the first. The arrow
// keys, Home, and End move through and open rows; Enter and Space choose a
// session or toggle the fold; other keys, such as Escape, pass on to the app.
import {useLayoutEffect, useRef, useState, type KeyboardEvent} from 'react';
import {chooseSession} from '../sessions/session-actions.ts';
import {expandSession} from '../app/workbench.ts';
import {shownId} from '../sessions/selection.ts';
import type {Session} from '../sessions/types.ts';
import {useStore} from '../store.ts';
import {EndedFold, SessionItem} from './session-item.tsx';

interface Row {
  id: string;
  session?: Session;
  level: 1 | 2;
  position: number;
  siblings: number;
  hasChildren: boolean;
  expanded: boolean;
  parent?: string;
  ended?: number;
}

type Unplaced = Omit<Row, 'position' | 'siblings'>;

interface SessionTreeProps {
  label: string;
  sessions: Session[];
  renaming: string | null;
  onRenaming: (id: string | null) => void;
}

export function SessionTree({label, sessions, renaming, onRenaming}: SessionTreeProps) {
  const [state, store] = useStore();
  const collapsed = state.workbench.collapsedSessions;
  const [focused, setFocused] = useState<string | null>(null);
  const [unfolded, setUnfolded] = useState<Record<string, boolean>>({});
  const ref = useRef<HTMLDivElement>(null);
  const hadFocus = useRef(false);

  // An active parent's ended children fold behind one row, except the shown
  // session, which stays listed while the fold is closed.
  const shown = shownId(state);
  const childRows = (parent: Session): Unplaced[] => {
    const child = (session: Session): Unplaced => ({id: session.id, session, level: 2, hasChildren: false, expanded: false, parent: parent.id});
    const ended = parent.ended ? [] : parent.children.filter(session => session.ended);
    if (!ended.length) return parent.children.map(child);
    const id = 'ended:' + parent.id;
    const open = !!unfolded[id];
    const listed = parent.children.filter(session => !session.ended || (!open && session.id === shown));
    const fold: Unplaced = {id, ended: ended.length, level: 2, hasChildren: true, expanded: open, parent: parent.id};
    return [...listed.map(child), fold, ...open ? ended.map(child) : []];
  };

  // The rows are flat, so each carries its level and its place among its
  // siblings for assistive technology.
  const rows = sessions.flatMap((session, index): Row[] => {
    const expanded = !collapsed[session.id];
    const nested = expanded ? childRows(session) : [];
    return [
      {id: session.id, session, level: 1, position: index + 1, siblings: sessions.length, hasChildren: session.children.length > 0, expanded},
      ...nested.map((row, i) => ({...row, position: i + 1, siblings: nested.length})),
    ];
  });
  const ids = rows.map(row => row.id);
  const active = ids.find(id => id === focused) ?? ids.find(id => id === shown) ?? ids[0];

  // When the focused row goes away, the focus moves to the tabbable row
  // instead of leaving the tree.
  hadFocus.current = !!ref.current?.contains(document.activeElement);
  useLayoutEffect(() => {
    const root = ref.current;
    if (hadFocus.current && root && !root.contains(document.activeElement)) root.querySelector<HTMLElement>('[tabindex="0"]')?.focus();
  });

  const setExpanded = (row: Row, open: boolean) => row.session
    ? store.update(s => expandSession(s, row.id, open))
    : setUnfolded(current => ({...current, [row.id]: open}));
  const focusRow = (id: string) => ref.current?.querySelector<HTMLElement>(`[data-row="${CSS.escape(id)}"]`)?.focus();

  const keyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    if (!(e.target instanceof HTMLElement)) return;
    const index = ids.indexOf(e.target.dataset.row ?? '');
    if (index < 0) return;
    const row = rows[index];
    const go = (i: number) => focusRow(ids[Math.max(0, Math.min(ids.length - 1, i))]);
    if (e.key === 'ArrowDown') go(index + 1);
    else if (e.key === 'ArrowUp') go(index - 1);
    else if (e.key === 'Home') go(0);
    else if (e.key === 'End') go(ids.length - 1);
    else if (e.key === 'ArrowRight' && row.hasChildren) row.expanded ? go(index + 1) : setExpanded(row, true);
    else if (e.key === 'ArrowLeft' && row.hasChildren && row.expanded) setExpanded(row, false);
    else if (e.key === 'ArrowLeft' && row.parent) focusRow(row.parent);
    else if (e.key === 'Enter' || e.key === ' ') row.session ? chooseSession(store, row.id) : setExpanded(row, !row.expanded);
    else return;
    e.preventDefault();
  };

  return (
    <div ref={ref} className="session-tree flex flex-col" role="tree" aria-label={label} onKeyDown={keyDown}>
      {rows.map(({session, ended, ...row}) => {
        const shared = {...row, tabbable: row.id === active, onToggle: () => setExpanded({session, ...row}, !row.expanded), onFocus: () => setFocused(row.id)};
        return session
          ? <SessionItem key={row.id} session={session} {...shared} renaming={renaming} onRenaming={onRenaming} />
          : <EndedFold key={row.id} count={ended ?? 0} {...shared} />;
      })}
    </div>
  );
}
