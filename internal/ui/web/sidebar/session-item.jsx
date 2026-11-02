// A session's row in the sidebar: its title, dimmed when it is stopped, and
// indicators naming its busy and waiting agents while it runs, and its unseen
// reply. Double-clicking the row renames the session in place.
import {useEffect, useRef} from 'preact/hooks';
import {renameSession} from '../sessions/session-actions.js';
import {StatusDot} from '../shared/status-dot.jsx';
import {useStore} from '../store.js';
import './session-item.css';

const names = agents => agents.map(a => a.name).join(', ');

export function SessionItem({session, selected, renaming, onRenaming}) {
  const busy = session.running ? names(session.agents.filter(a => a.activity === 'busy')) : '';
  const waiting = session.running ? names(session.agents.filter(a => a.attention)) : '';
  const unseen = session.unseen && !selected;
  return (
    <vscode-tree-item
      data-session={session.id}
      selected={selected}
      class={session.running ? 'session-item' : 'session-item session-item-stopped'}
      ondblclick={() => onRenaming(true)}
    >
      {renaming
        ? <RenameField session={session} onDone={() => onRenaming(false)} />
        : <span class="session-item-title">{session.title}</span>}
      {(busy || waiting || unseen) && (
        <span slot="decoration" class="session-item-indicators">
          {busy && <vscode-icon name="loading" spin role="img" title={'Busy: ' + busy} />}
          {waiting && <StatusDot kind="waiting" title={'Waiting on you: ' + waiting} />}
          {unseen && <StatusDot kind="unseen" title="New reply" />}
        </span>
      )}
    </vscode-tree-item>
  );
}

// RenameField saves a trimmed, nonempty, changed title on Enter or when it
// loses the focus, and cancels on Escape. It keeps its keys and clicks from
// the tree, which would treat them as navigation and selection.
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
