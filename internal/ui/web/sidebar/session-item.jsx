// A session's tree row. Its right edge shows the worktree branch icon and,
// only when there is something to say, the session's own state.
import {useEffect, useRef} from 'preact/hooks';
import {mergeTint} from '../sessions/merge.js';
import {renameSession} from '../sessions/session-actions.js';
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

// nested lists the child rows to draw beneath a parent.
export function SessionItem({session, nested = [], renaming, onRenaming}) {
  const [state] = useStore();
  const selected = session.id === state.selected;
  const classes = ['session-item', !session.running && 'session-item-stopped'].filter(Boolean).join(' ');

  return (
    <vscode-tree-item class={classes} data-session={session.id} branch={nested.length > 0} open selected={selected}>
      {session.id === renaming
        ? <RenameField session={session} onDone={() => onRenaming(null)} />
        : session.title}
      <span slot="decoration" class="session-item-decoration">
        {session.worktree && <BranchIcon session={session} />}
        <span class="session-item-status">{statusOf(session, selected)}</span>
      </span>
      {nested.map(child => <SessionItem key={child.id} session={child} renaming={renaming} onRenaming={onRenaming} />)}
    </vscode-tree-item>
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
    const row = ref.current.closest('vscode-tree-item');
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
      onfocusout={() => finish(true)}
    />
  );
}
