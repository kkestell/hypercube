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
  const classes = `session-item h-7 justify-start pl-2 pr-0 font-normal ${level === 2 ? 'ml-4' : ''} ${stopped(session) ? 'opacity-60' : ''}`;
  const toggle = (e: MouseEvent) => {
    e.stopPropagation();
    onToggle();
  };

  return (
    <Button asChild variant={selected ? 'secondary' : 'ghost'} size="sm"><div
      className={classes} role="treeitem" data-session={session.id}
      aria-level={level} aria-posinset={position} aria-setsize={siblings} aria-expanded={hasChildren ? expanded : undefined} aria-selected={selected}
      tabIndex={tabbable ? 0 : -1} onFocus={onFocus}
      onClick={() => chooseSession(store, session.id)} onDoubleClick={hasChildren ? toggle : undefined}
    >
      <span className="session-item-twistie flex size-4 shrink-0 items-center" onClick={hasChildren ? toggle : undefined}>
        {hasChildren && <Icon name={expanded ? 'chevron-down' : 'chevron-right'} />}
      </span>
      {session.id === renaming
        ? <RenameField session={session} onDone={() => onRenaming(null)} />
        : <span className={`session-item-title min-w-0 flex-1 truncate text-left ${level === 2 ? 'text-xs' : ''} ${session.activity === 'busy' ? 'text-foreground' : 'text-neutral-500'}`}>{session.title}</span>}
      <span className="session-item-decoration ml-auto flex shrink-0 items-center gap-1">
        {session.worktree && <BranchIcon className="session-item-branch w-7 justify-center" merge={session.merge} />}
        <span className="session-item-status inline-flex h-4 w-7 items-center justify-center">{statusOf(session, selected)}</span>
      </span>
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
