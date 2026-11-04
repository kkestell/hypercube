// A session's row: its title, dimmed for a stopped session, followed for a
// worktree session by a branch icon, tinted while its branch has work outside
// the checkout's branch and dimmed once merged; and its own state dot: amber
// while it waits on the user, blue for a reply not yet seen, and otherwise a
// gray ring while that session works.
// Hovering shows its menu button in the indicator's place. Double-clicking
// the title renames the session in place.
import {useEffect, useRef} from 'preact/hooks';
import {mergeTint} from '../sessions/merge.js';
import {chooseSession, renameSession} from '../sessions/session-actions.js';
import {StatusDot} from '../shared/status-dot.jsx';
import {useStore} from '../store.js';
import './session-item.css';

const dotOf = (session, selected) => {
  if (session.attention) return <StatusDot kind="waiting" title="Waiting on you" />;
  if (session.unseen && !selected) return <StatusDot kind="unseen" title="New reply" />;
  if (session.activity === 'busy') return <StatusDot kind="busy" title="Working" />;
  return null;
};

// onMenu is called with the menu button when the session has a menu.
export function SessionItem({session, workspace, selected, renaming, onRenaming, onMenu}) {
  const [, store] = useStore();
  const classes = ['session-item', session.parent ? 'session-item-nested' : 'session-item-parent'];
  if (selected) classes.push('session-item-selected');
  if (!session.running) classes.push('session-item-stopped');

  return (
    <div class={classes.join(' ')} data-session={session.id}>
      {renaming
        ? <RenameField session={session} onDone={() => onRenaming(false)} />
        : (
          <button class="session-item-title" aria-current={selected ? 'true' : undefined} onClick={() => chooseSession(store, session.id)} ondblclick={() => onRenaming(true)}>
            {workspace && <span class="session-item-workspace" title={workspace.path}>{workspace.name}</span>}
            <span class="session-item-text">{session.title}</span>
          </button>
        )}
      {session.worktree && <BranchIcon session={session} />}
      <span class="session-item-status">
        {dotOf(session, selected)}
        {onMenu && (
          <button class="session-item-menu" title="More Actions" aria-label="More Actions" onClick={e => onMenu(e.currentTarget)}>
            <vscode-icon name="ellipsis" />
          </button>
        )}
      </span>
    </div>
  );
}

function BranchIcon({session}) {
  const tint = mergeTint(session.merge);
  const classes = ['session-item-branch', tint && `session-item-branch-${tint}`].filter(Boolean).join(' ');
  // Slightly smaller than the row's other icons.
  return <vscode-icon class={classes} name="git-branch" size="14" title={session.merge?.branch} />;
}

// RenameField saves a trimmed, nonempty, changed title on Enter or when it
// loses the focus, and cancels on Escape. It keeps its keys and clicks from
// the sidebar.
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
    if (e.key === 'Enter') finish(true);
    else if (e.key === 'Escape') finish(false);
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
