// A workspace's sessions as a keyboard-navigable tree, with each parent's visible
// children following it. An active parent lists its ended children only when
// its menu shows them, and always lists the shown session. One row is
// tabbable: the focused row, else the selected one, else the first. The arrow
// keys, Home, and End move through and open rows; Enter and Space choose a
// session; other keys, such as Escape, pass on to the app.
import {useLayoutEffect, useRef, useState, type KeyboardEvent} from 'react';
import {chooseSession} from '../sessions/session-actions.ts';
import {expandSession} from '../app/workbench.ts';
import {shownId} from '../sessions/selection.ts';
import type {Session} from '../sessions/types.ts';
import {useStore} from '../store.ts';
import {SessionItem} from './session-item.tsx';

interface Row {
  id: string;
  session: Session;
  level: 1 | 2;
  position: number;
  siblings: number;
  hasChildren: boolean;
  expanded: boolean;
  parent?: string;
}

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
  const ref = useRef<HTMLDivElement>(null);
  const hadFocus = useRef(false);

  const shown = shownId(state);
  const listed = (parent: Session) => parent.ended || state.endedShown[parent.id]
    ? parent.children
    : parent.children.filter(child => !child.ended || child.id === shown);

  // The rows are flat, so each carries its level and its place among its
  // siblings for assistive technology.
  const rows = sessions.flatMap((session, index): Row[] => {
    const children = listed(session);
    const expanded = !collapsed[session.id];
    return [
      {id: session.id, session, level: 1, position: index + 1, siblings: sessions.length, hasChildren: children.length > 0, expanded},
      ...expanded ? children.map((child, i): Row => ({id: child.id, session: child, level: 2, position: i + 1, siblings: children.length, hasChildren: false, expanded: false, parent: session.id})) : [],
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

  const setExpanded = (row: Row, open: boolean) => store.update(s => expandSession(s, row.id, open));
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
    else if (e.key === 'Enter' || e.key === ' ') chooseSession(store, row.id);
    else return;
    e.preventDefault();
  };

  return (
    <div ref={ref} className="session-tree flex flex-col" role="tree" aria-label={label} onKeyDown={keyDown}>
      {rows.map(row => <SessionItem key={row.id} session={row.session} level={row.level} position={row.position} siblings={row.siblings}
        hasChildren={row.hasChildren} expanded={row.expanded} tabbable={row.id === active}
        onToggle={() => setExpanded(row, !row.expanded)} onFocus={() => setFocused(row.id)} renaming={renaming} onRenaming={onRenaming} />)}
    </div>
  );
}
