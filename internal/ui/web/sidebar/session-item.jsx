// A session's tree row. Its right edge shows the worktree branch icon and,
// only when there is something to say, the session's own state. A click
// chooses the session; a click on a parent's chevron, or a double-click on the
// parent, collapses or expands its children.
import {useEffect, useRef} from 'preact/hooks';
import {mergeTint} from '../sessions/merge.js';
import {chooseSession, renameSession} from '../sessions/session-actions.js';
import {StatusDot} from '../shared/status-dot.jsx';
import {useStore} from '../store.js';
import './session-item.css';

const statusOf = (session, selected) => {
  if (session.activity === 'exited') return <StatusDot kind="exited" title="Exited" />;
  if (session.attention) return <StatusDot kind="waiting" title="Waiting on you" />;
  if (session.unseen && !selected) return <StatusDot kind="unseen" title="New reply" />;
  if (session.activity === 'busy') return <vscode-icon class="session-item-busy" name="loading" spin role="img" title="Working" />;
  return null;
};

export function SessionItem({session, level, hasChildren, expanded, tabbable, onToggle, onFocus, renaming, onRenaming}) {
  const [state, store] = useStore();
  const selected = session.id === state.selected;
  const classes = ['session-item', !session.running && 'session-item-stopped'].filter(Boolean).join(' ');
  const toggle = e => {
    e.stopPropagation();
    onToggle();
  };

  return (
    <div
      class={classes} role="treeitem" data-session={session.id} style={{'--session-item-level': level}}
      aria-level={level} aria-expanded={hasChildren ? expanded : undefined} aria-selected={selected}
      tabIndex={tabbable ? 0 : -1} onFocus={onFocus}
      onClick={() => chooseSession(store, session.id)} onDblClick={hasChildren ? toggle : undefined}
    >
      <span class="session-item-twistie" onClick={hasChildren ? toggle : undefined}>
        {hasChildren && <vscode-icon name={expanded ? 'chevron-down' : 'chevron-right'} />}
      </span>
      {session.id === renaming
        ? <RenameField session={session} onDone={() => onRenaming(null)} />
        : <span class="session-item-title">{session.title}</span>}
      <span class="session-item-decoration">
        {session.worktree && <BranchIcon session={session} />}
        <span class="session-item-status">{statusOf(session, selected)}</span>
      </span>
    </div>
  );
}

function BranchIcon({session}) {
  const tint = mergeTint(session.merge);
  const classes = ['session-item-branch', tint && `session-item-branch-${tint}`].filter(Boolean).join(' ');
  return <vscode-icon class={classes} name="git-branch" title={session.merge?.branch} />;
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
    field.updateComplete?.then(() => field.wrappedElement.select());
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
    <vscode-textfield
      ref={ref}
      class="session-item-rename"
      label="Session title"
      value={session.title}
      onKeyDown={key}
      onClick={e => e.stopPropagation()}
      onDblClick={e => e.stopPropagation()}
      onfocusout={() => finish(true)}
    />
  );
}
