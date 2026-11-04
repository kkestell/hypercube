// A session's tree row. Its right edge shows the worktree branch icon and,
// only when there is something to say, the session's own state. A click
// chooses the session; a click on a parent's chevron, or a double-click on the
// parent, collapses or expands its children.
import {useEffect, useRef} from 'react';
import {mergeSummary} from '../sessions/merge.js';
import {chooseSession, renameSession} from '../sessions/session-actions.js';
import {StatusIcon} from '../shared/status-icon.jsx';
import {useStore} from '../store.js';
import {Button} from '../components/ui/button';
import {Input} from '../components/ui/input';
import {Icon} from '../shared/icon.jsx';

const statusOf = (session, selected) => {
  if (session.activity === 'exited') return <StatusIcon kind="exited" title="Exited" />;
  if (session.attention) return <StatusIcon kind="waiting" title="Waiting on you" />;
  if (session.unseen && !selected) return <StatusIcon kind="unseen" title="New reply" />;
  if (session.activity === 'busy') return <Icon className="session-item-busy" name="loading" spin role="img" title="Working" />;
  return null;
};

export function SessionItem({session, level, position, siblings, hasChildren, expanded, tabbable, onToggle, onFocus, renaming, onRenaming}) {
  const [state, store] = useStore();
  const selected = session.id === state.selected;
  const classes = `session-item w-full justify-start ${level === 2 ? 'pl-7' : ''} ${!session.running || session.ended ? 'opacity-60' : ''}`;
  const toggle = e => {
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
        : <span className="session-item-title min-w-0 flex-1 truncate text-left">{session.title}</span>}
      <span className="session-item-decoration ml-auto flex shrink-0 items-center gap-1.5">
        {session.worktree && <BranchIcon session={session} />}
        <span className="session-item-status inline-flex size-4 items-center justify-center">{statusOf(session, selected)}</span>
      </span>
    </div></Button>
  );
}

function BranchIcon({session}) {
  const title = session.merge ? `${session.merge.branch}: ${mergeSummary(session.merge)}` : 'Worktree';
  return <span className="session-item-branch inline-flex text-muted-foreground" title={title}>
    <Icon name="git-branch" title={title} role="img" />
  </span>;
}

// RenameField saves a trimmed, nonempty, changed title on Enter or when it
// loses the focus, and cancels on Escape. Enter and Escape return the focus
// to the row. It keeps its keys and clicks from the tree.
function RenameField({session, onDone}) {
  const [, store] = useStore();
  const ref = useRef(null);
  const finished = useRef(false);

  useEffect(() => {
    const field = ref.current;
    field.focus();
    field.select();
  }, []);

  const finish = save => {
    if (finished.current) return;
    finished.current = true;
    const title = save ? ref.current.value.trim() : '';
    if (title && title !== session.title) renameSession(store, session.id, title);
    onDone();
  };

  const key = e => {
    e.stopPropagation();
    if (e.key !== 'Enter' && e.key !== 'Escape') return;
    const row = ref.current.closest('[role="treeitem"]');
    finish(e.key === 'Enter');
    row.focus();
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
