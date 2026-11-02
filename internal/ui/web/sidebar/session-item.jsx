// A session's row in the sidebar: its title, dimmed when it is stopped, a caret
// when it has nested sessions, and a dot for its own state: amber while it
// waits on the user, blue for a reply the user has not seen. Hovering the row
// shows its menu button in the dot's place. Double-clicking the title renames
// the session in place.
import {useEffect, useRef} from 'preact/hooks';
import {chooseSession, renameSession} from '../sessions/session-actions.js';
import {StatusDot} from '../shared/status-dot.jsx';
import {useStore} from '../store.js';
import './session-item.css';

const dotOf = (session, selected) => {
  if (session.running && session.attention) return <StatusDot kind="waiting" title="Waiting on you" />;
  if (session.unseen && !selected) return <StatusDot kind="unseen" title="New reply" />;
  return null;
};

// onToggle is set when the session has nested sessions, and onMenu, called
// with the menu button, when it has a menu.
export function SessionItem({session, selected, renaming, onRenaming, open, onToggle, onMenu}) {
  const [, store] = useStore();
  const classes = ['session-item'];
  if (session.parent) classes.push('session-item-nested');
  if (selected) classes.push('session-item-selected');
  if (!session.running) classes.push('session-item-stopped');

  return (
    <div class={classes.join(' ')} data-session={session.id}>
      {renaming
        ? <RenameField session={session} onDone={() => onRenaming(false)} />
        : (
          <button class="session-item-title" onClick={() => chooseSession(store, session.id)} ondblclick={() => onRenaming(true)}>
            <span class="session-item-text">{session.title}</span>
          </button>
        )}
      {onToggle && !renaming && (
        <button class="session-item-caret" aria-expanded={open} title={open ? 'Collapse' : 'Expand'} onClick={onToggle}>
          <vscode-icon name={open ? 'chevron-down' : 'chevron-right'} />
        </button>
      )}
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
      value={session.title}
      onKeyDown={key}
      onClick={e => e.stopPropagation()}
      onfocusout={() => finish(true)}
    />
  );
}
