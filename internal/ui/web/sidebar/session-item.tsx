// A session's tree row. A parent shows a chevron after its title on hover or
// focus. The row's right edge reserves columns for settings differences,
// worktree branch state, and session status. A click chooses the
// session; a click on a parent's chevron, or a double-click on the parent,
// collapses or expands its children. EndedFold is the child row that hides or
// lists a parent's ended children.
import {useEffect, useRef, type KeyboardEvent, type MouseEvent} from 'react';
import {chooseSession, renameSession} from '../sessions/session-actions.ts';
import {sessionOf, shownId} from '../sessions/selection.ts';
import {SettingsDiffer} from '../agent/session-selection.tsx';
import type {Session} from '../sessions/types.ts';
import {BranchIcon} from '../shared/branch-icon.tsx';
import {StatusDot} from '../shared/status-dot.tsx';
import {useStore} from '../store.ts';
import {Button} from '../components/ui/button';
import {Input} from '../components/ui/input';
import {BusyDot} from '../shared/busy-dot.tsx';
import {Chevron} from '../shared/icon.tsx';

const statusOf = (session: Session, selected: boolean, waiting: boolean) => {
  if (session.ended) return null;
  if (waiting) return <StatusDot kind="waiting" title="Waiting on you" />;
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
  const parent = level === 2 ? sessionOf(state, session.id)?.parent : null;
  const selected = session.id === shownId(state);
  const status = statusOf(session, selected, session.attention || state.requests.some(request => request.session === session.id));
  const classes = `session-item group/session relative h-7 cursor-default justify-start px-2 font-normal ${selected ? 'hover:bg-secondary' : ''} ${level === 2 ? 'ml-5' : ''} ${session.ended ? 'opacity-60' : ''}`;
  const toggle = (e: MouseEvent) => {
    e.stopPropagation();
    onToggle();
  };

  return (
    <Button asChild variant={selected ? 'secondary' : 'ghost'} size="sm" className={classes}><div
      role="treeitem" data-row={session.id} data-session={session.id}
      aria-level={level} aria-posinset={position} aria-setsize={siblings} aria-expanded={hasChildren ? expanded : undefined} aria-selected={selected}
      tabIndex={tabbable ? 0 : -1} onFocus={onFocus}
      onClick={() => chooseSession(store, session.id)} onDoubleClick={hasChildren ? toggle : undefined}
    >
      {level === 2 && <Branch position={position} siblings={siblings} />}
      <span className="flex min-w-0 flex-1 items-center gap-2">
        {session.id === renaming
          ? <RenameField session={session} onDone={() => onRenaming(null)} />
          : <span className="relative min-w-0 max-w-full">
            <span className={`session-item-title block truncate text-left ${level === 2 ? 'text-xs' : ''}`}>{session.title}</span>
            {hasChildren && <span className="session-item-twistie invisible absolute right-0 top-1/2 flex size-4 -translate-y-1/2 items-center group-hover/session:visible group-focus-within/session:visible" onClick={toggle}>
              <Chevron open={expanded} />
            </span>}
          </span>}
      </span>
      {session.id !== renaming && <span className="session-item-decoration flex shrink-0 items-center gap-1">
        <span className="inline-flex size-4 items-center justify-center">
          {parent && <SettingsDiffer child={session} parent={parent} workspace={sessionOf(state, session.id)?.workspace.id ?? null} />}
        </span>
        <span className="inline-flex size-4 items-center justify-center">
          {session.worktree && <BranchIcon className="session-item-branch" merge={session.merge} />}
        </span>
        <span className="session-item-status inline-flex size-4 items-center justify-center">{status}</span>
      </span>}
    </div></Button>
  );
}

// Branch draws a child row's stretch of the line down from its parent.
function Branch({position, siblings}: {position: number; siblings: number}) {
  return <span aria-hidden="true" className={`pointer-events-none absolute -left-2 border-l border-border ${position === 1 ? 'top-1' : 'top-0'} ${position === siblings ? 'bottom-1/2 w-1 rounded-bl-sm border-b' : 'bottom-0'}`} />;
}

interface EndedFoldProps {
  id: string;
  count: number;
  position: number;
  siblings: number;
  expanded: boolean;
  tabbable: boolean;
  onToggle: () => void;
  onFocus: () => void;
}

export function EndedFold({id, count, position, siblings, expanded, tabbable, onToggle, onFocus}: EndedFoldProps) {
  return (
    <Button asChild variant="ghost" size="sm" className="session-fold relative ml-5 h-7 cursor-default justify-start px-2 text-xs font-normal text-muted-foreground"><div
      role="treeitem" data-row={id} aria-level={2} aria-posinset={position} aria-setsize={siblings} aria-expanded={expanded} aria-selected={false}
      tabIndex={tabbable ? 0 : -1} onFocus={onFocus} onClick={onToggle}
    >
      <Branch position={position} siblings={siblings} />
      <span className="flex items-center gap-1">{count} ended<Chevron open={expanded} /></span>
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
