// A workspace's sessions as a keyboard-navigable tree, with each parent's visible
// children one level in. One row is tabbable: the focused row, else the
// selected one, else the first. The arrow keys, Home, and End move through
// and open rows; Enter and Space choose the row; other keys,
// such as Escape, pass on to the app.
import {useLayoutEffect, useRef, useState} from 'react';
import {chooseSession} from '../sessions/session-actions.js';
import {useStore} from '../store.js';
import {SessionItem} from './session-item.jsx';

export function SessionTree({label, sessions, renaming, onRenaming}) {
  const [state, store] = useStore();
  const [collapsed, setCollapsed] = useState({});
  const [focused, setFocused] = useState(null);
  const ref = useRef(null);
  const hadFocus = useRef(false);

  // The rows are flat, so each carries its level and its place among its
  // siblings for assistive technology.
  const rows = sessions.flatMap((session, index) => {
    const nested = session.children;
    const expanded = !collapsed[session.id];
    const row = {session, level: 1, position: index + 1, siblings: sessions.length, hasChildren: nested.length > 0, expanded};
    const children = nested.map((child, i) => ({session: child, level: 2, position: i + 1, siblings: nested.length, parent: session.id}));
    return [row, ...expanded ? children : []];
  });
  const ids = rows.map(row => row.session.id);
  const active = [focused, state.selected].find(id => ids.includes(id)) ?? ids[0];

  // When the focused row goes away, the focus moves to the tabbable row
  // instead of leaving the tree.
  hadFocus.current = !!ref.current?.contains(document.activeElement);
  useLayoutEffect(() => {
    if (hadFocus.current && !ref.current.contains(document.activeElement)) ref.current.querySelector('[tabindex="0"]')?.focus();
  });

  const setExpanded = (id, expanded) => setCollapsed(c => ({...c, [id]: !expanded}));
  const focusRow = id => ref.current.querySelector(`[data-session="${CSS.escape(id)}"]`).focus();

  const keyDown = e => {
    const index = ids.indexOf(e.target.dataset.session);
    if (index < 0) return;
    const row = rows[index];
    const go = i => focusRow(ids[Math.max(0, Math.min(ids.length - 1, i))]);
    if (e.key === 'ArrowDown') go(index + 1);
    else if (e.key === 'ArrowUp') go(index - 1);
    else if (e.key === 'Home') go(0);
    else if (e.key === 'End') go(ids.length - 1);
    else if (e.key === 'ArrowRight' && row.hasChildren) row.expanded ? go(index + 1) : setExpanded(row.session.id, true);
    else if (e.key === 'ArrowLeft' && row.hasChildren && row.expanded) setExpanded(row.session.id, false);
    else if (e.key === 'ArrowLeft' && row.parent) focusRow(row.parent);
    else if (e.key === 'Enter' || e.key === ' ') chooseSession(store, row.session.id);
    else return;
    e.preventDefault();
  };

  return (
    <div ref={ref} className="session-tree" role="tree" aria-label={label} onKeyDown={keyDown}>
      {rows.map(row => (
        <SessionItem
          key={row.session.id} {...row} tabbable={row.session.id === active}
          onToggle={() => setExpanded(row.session.id, !row.expanded)} onFocus={() => setFocused(row.session.id)}
          renaming={renaming} onRenaming={onRenaming}
        />
      ))}
    </div>
  );
}
