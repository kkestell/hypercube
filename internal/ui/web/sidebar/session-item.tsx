// A session's tree row. Its right edge shows the worktree branch icon and,
// only when there is something to say, the session's own state. A click
// chooses the session; a click on a parent's chevron, or a double-click on the
// parent, collapses or expands its children.
import {useEffect, useRef, type KeyboardEvent, type MouseEvent} from 'react';
import {chooseSession, renameSession} from '../sessions/session-actions.ts';
import {stopped} from '../sessions/selection.ts';
import type {Session} from '../sessions/types.ts';
import {BranchIcon} from '../shared/branch-icon.tsx';
import {StatusDot} from '../shared/status-dot.tsx';
import {useStore} from '../store.ts';
import {Button} from '../components/ui/button';
import {Input} from '../components/ui/input';
import {BusyDot} from '../shared/busy-dot.tsx';
import {Icon} from '../shared/icon.tsx';

const statusOf = (session: Session, selected: boolean) => {
  if (stopped(session)) return null;
  if (session.attention) return <StatusDot kind="waiting" title="Waiting on you" />;
  if (session.unseen && !selected) return <StatusDot kind="unseen" title="New reply" />;
  if (session.activity === 'busy') return <BusyDot title="Working" />;
  return null;
};

interface SessionItemProps {
  session: Session;
  level: 1 | 2;
  position: number;
  siblings: number;
  hasChildren: boolean;
  expanded: boolean;
  tabbable: boolean;
  onToggle: () => void;
  onFocus: () => void;
  renaming: string | null;
  onRenaming: (id: string | null) => void;
}

export function SessionItem({session, level, position, siblings, hasChildren, expanded, tabbable, onToggle, onFocus, renaming, onRenaming}: SessionItemProps) {
  const [state, store] = useStore();
  const selected = session.id === state.selected;
  const status = statusOf(session, selected);
  const background = selected
    ? '[--row-bg:var(--secondary)] hover:bg-secondary'
    : '[--row-bg:var(--card)] hover:bg-transparent dark:hover:bg-transparent';
  const classes = `session-item group/session relative h-7 cursor-default justify-start px-2 font-normal ${background} ${level === 2 ? 'ml-4' : ''} ${stopped(session) ? 'opacity-60' : ''}`;
  const toggle = (e: MouseEvent) => {
    e.stopPropagation();
    onToggle();
  };

  return (
    <Button asChild variant={selected ? 'secondary' : 'ghost'} size="sm" className={classes}><div
      role="treeitem" data-session={session.id}
      aria-level={level} aria-posinset={position} aria-setsize={siblings} aria-expanded={hasChildren ? expanded : undefined} aria-selected={selected}
      tabIndex={tabbable ? 0 : -1} onFocus={onFocus}
      onClick={() => chooseSession(store, session.id)} onDoubleClick={hasChildren ? toggle : undefined}
    >
      <span className="flex min-w-0 flex-1 items-center">
        {session.id === renaming
          ? <RenameField session={session} onDone={() => onRenaming(null)} />
          : <span className={`session-item-title min-w-0 truncate text-left ${level === 2 ? 'text-xs' : ''}`}>{session.title}</span>}
      </span>
      {session.id !== renaming && (hasChildren || session.worktree || status) && <span className={`absolute inset-y-0 right-0 flex items-center gap-1 rounded-r-md bg-linear-to-r from-transparent via-(--row-bg) via-25% to-(--row-bg) pl-4 pr-2 ${!session.worktree && !status ? 'invisible group-hover/session:visible group-focus-within/session:visible' : ''}`}>
        {hasChildren && <span className="session-item-twistie hidden size-4 shrink-0 items-center group-hover/session:flex group-focus-within/session:flex" onClick={toggle}>
          <Icon name={expanded ? 'chevron-down' : 'chevron-right'} />
        </span>}
        {(session.worktree || status) && <span className="session-item-decoration flex shrink-0 items-center gap-1">
          {session.worktree && <BranchIcon className="session-item-branch" merge={session.merge} />}
          {status && <span className="session-item-status inline-flex size-4 items-center justify-center">{status}</span>}
        </span>}
      </span>}
    </div></Button>
  );
}

// RenameField saves a trimmed, nonempty, changed title on Enter or when it
// loses the focus, and cancels on Escape. Enter and Escape return the focus
// to the row. It keeps its keys and clicks from the tree.
function RenameField({session, onDone}: {session: Session; onDone: () => void}) {
  const [, store] = useStore();
  const ref = useRef<HTMLInputElement>(null);
  const finished = useRef(false);

  useEffect(() => {
    const field = ref.current;
    if (!field) return;
    field.focus();
    field.select();
  }, []);

  const finish = (save: boolean) => {
    if (finished.current) return;
    finished.current = true;
    const title = save ? ref.current?.value.trim() : '';
    if (title && title !== session.title) renameSession(store, session.id, title);
    onDone();
  };

  const key = (e: KeyboardEvent<HTMLInputElement>) => {
    e.stopPropagation();
    if (e.key !== 'Enter' && e.key !== 'Escape') return;
    const row = e.currentTarget.closest<HTMLElement>('[role="treeitem"]');
    finish(e.key === 'Enter');
    row?.focus();
  };

  return (
    <Input
      ref={ref}
      className="session-item-rename"
      aria-label="Session title"
      defaultValue={session.title}
      onKeyDown={key}
      onClick={e => e.stopPropagation()}
      onDoubleClick={e => e.stopPropagation()}
      onBlur={() => finish(true)}
    />
  );
}
