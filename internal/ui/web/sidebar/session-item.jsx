// A session's row in the sidebar: its title, dimmed when it is stopped, and
// indicators naming its busy and waiting sessions while it runs, and its unseen
// reply. Double-clicking the row renames the session in place.
import {useEffect, useRef} from 'preact/hooks';
import {chooseSession, renameSession} from '../sessions/session-actions.js';
import {StatusDot} from '../shared/status-dot.jsx';
import {useStore} from '../store.js';
import './session-item.css';

export function SessionItem({session, selected, renaming, onRenaming, open, children}) {
  const [, store] = useStore();
  const row = useRef(null);
  const branch = session.children.length > 0;
  const busy = session.running && session.activity === 'busy';
  const waiting = [session, ...session.children].filter(s => s.running && s.attention).map(s => s.title).join(', ');
  const unseen = !session.parent && session.unseen && !selected;

  // vscode-tree toggles branches on every row click. Own parent clicks so
  // only the chevron changes expansion; child rows retain their own behavior.
  useEffect(() => {
    if (!branch) return;
    const element = row.current;
    const clicked = event => {
      const path = event.composedPath();
      if (path.find(node => node.localName === 'vscode-tree-item') !== element) return;
      event.stopPropagation();
      if (renaming) return;
      if (path.some(node => node.getAttribute?.('part')?.split(' ').includes('arrow-icon-container'))) element.toggleAttribute('open');
      else chooseSession(store, session.id);
    };
    element.addEventListener('click', clicked, true);
    return () => element.removeEventListener('click', clicked, true);
  }, [branch, renaming, session.id, store]);

  return (
    <vscode-tree-item
      ref={row} data-session={session.id} data-key={branch ? session.id : undefined}
      branch={branch} open={branch && open} selected={selected}
      class={session.running ? 'session-item' : 'session-item session-item-stopped'}
      ondblclick={e => { e.stopPropagation(); onRenaming(true); }}
    >
      {renaming
        ? <RenameField session={session} onDone={() => onRenaming(false)} />
        : <span class="session-item-title">{session.title}</span>}
      {(busy || waiting || unseen) && (
        <span slot="decoration" class="session-item-indicators">
          {busy && <vscode-icon name="loading" spin role="img" title={'Busy: ' + session.title} />}
          {waiting && <StatusDot kind="waiting" title={'Waiting on you: ' + waiting} />}
          {unseen && <StatusDot kind="unseen" title="New reply" />}
        </span>
      )}
      {children}
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
