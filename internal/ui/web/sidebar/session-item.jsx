// A session's own state sits at the far right, beside its worktree branch icon.
// double-clicking a title renames it in place.
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
  return <StatusDot kind="idle" title={session.running ? 'Idle' : 'Stopped'} />;
};

export function SessionItem({session, selected, renaming, onRenaming, hasChildren, expanded, onToggle}) {
  const [, store] = useStore();
  const classes = ['session-item', session.parent ? 'session-item-nested' : 'session-item-parent'];
  const Title = renaming ? 'div' : 'button';
  if (selected) classes.push('session-item-selected');
  if (!session.running) classes.push('session-item-stopped');

  return (
    <div class={classes.join(' ')} data-session={session.id}>
      {hasChildren
        ? <button class="session-item-toggle" aria-label={`${expanded ? 'Collapse' : 'Expand'} ${session.title}`} aria-expanded={expanded} onClick={onToggle}><vscode-icon name={expanded ? 'chevron-down' : 'chevron-right'} /></button>
        : <span class="session-item-toggle" />}
      <Title class="session-item-title" aria-current={selected ? 'true' : undefined}
        onClick={renaming ? undefined : () => chooseSession(store, session.id)}
        ondblclick={renaming ? undefined : () => onRenaming(true)}>
        <span class="session-item-title-line">
          {renaming
            ? <RenameField session={session} onDone={() => onRenaming(false)} />
            : <span class="session-item-text">{session.title}</span>}
        </span>
      </Title>
      <span class="session-item-actions">
        <span class="session-item-branch-slot">{session.worktree && <BranchIcon session={session} />}</span>
        <span class="session-item-status">{dotOf(session, selected)}</span>
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
