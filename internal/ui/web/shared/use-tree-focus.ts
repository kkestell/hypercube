// Keyboard focus for a tree of rows. One row is tabbable: the focused row,
// else the selected one, else the first. When the focused row goes away while
// the tree has focus, the focus moves to the tabbable row instead of leaving
// the tree. The arrow keys, Home, and End move through rows; Right opens a
// closed row or moves into an open one, and Left closes an open row or moves
// to its parent. Each row's element carries its ID in data-row.
import {useLayoutEffect, useRef, useState, type FocusEvent, type KeyboardEvent, type RefObject} from 'react';

export interface TreeRow {
  id: string;
  expandable: boolean;
  expanded: boolean;
  parent?: string;
}

export function useTreeFocus(tree: RefObject<HTMLElement | null>, rows: TreeRow[], selected: string | null | undefined, setExpanded: (id: string, open: boolean) => void) {
  const [focused, setFocused] = useState<string | null>(null);
  const inside = useRef(false);
  const ids = rows.map(row => row.id);
  const tabbable = ids.find(id => id === focused) ?? ids.find(id => id === selected) ?? ids[0];

  const focusRow = (id: string | undefined) => {
    if (id !== undefined) tree.current?.querySelector<HTMLElement>(`[data-row="${CSS.escape(id)}"]`)?.focus();
  };

  useLayoutEffect(() => {
    if (inside.current && focused !== null && !ids.includes(focused) && document.activeElement === document.body) focusRow(tabbable);
  });

  const onFocus = (e: FocusEvent<HTMLElement>) => {
    inside.current = true;
    const row = e.target.closest<HTMLElement>('[data-row]');
    if (row?.dataset.row !== undefined) setFocused(row.dataset.row);
  };
  // A focused row removed from the page blurs toward nothing, which leaves
  // the tree in charge of the focus.
  const onBlur = (e: FocusEvent<HTMLElement>) => {
    if (e.relatedTarget instanceof Node && !e.currentTarget.contains(e.relatedTarget)) inside.current = false;
  };

  // keyDown handles a movement key on a row and reports whether it did.
  const keyDown = (e: KeyboardEvent<HTMLElement>) => {
    if (!(e.target instanceof Element)) return false;
    const index = ids.indexOf(e.target.closest<HTMLElement>('[data-row]')?.dataset.row ?? '');
    if (index < 0) return false;
    const row = rows[index];
    const go = (i: number) => focusRow(ids[Math.max(0, Math.min(ids.length - 1, i))]);
    if (e.key === 'ArrowDown') go(index + 1);
    else if (e.key === 'ArrowUp') go(index - 1);
    else if (e.key === 'Home') go(0);
    else if (e.key === 'End') go(ids.length - 1);
    else if (e.key === 'ArrowRight' && row.expandable) {
      if (row.expanded) go(index + 1);
      else setExpanded(row.id, true);
    } else if (e.key === 'ArrowLeft' && row.expandable && row.expanded) setExpanded(row.id, false);
    else if (e.key === 'ArrowLeft' && row.parent !== undefined) focusRow(row.parent);
    else return false;
    e.preventDefault();
    return true;
  };

  return {tabbable, treeProps: {onFocus, onBlur}, keyDown};
}
