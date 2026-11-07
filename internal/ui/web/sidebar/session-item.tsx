// A session's tree row. A parent shows a chevron after its title on hover or
// keyboard focus. Before the title, a fixed slot shows the session's state.
// The row's right edge shows the worktree branch icon and, for a child, how its
// settings differ. A click chooses the session; a click on a parent's chevron,
// or a double-click on the parent, collapses or expands its children.
// EndedFold is the child row that hides or lists a parent's ended children.
import {useEffect, useLayoutEffect, useRef, useState, type KeyboardEvent, type MouseEvent, type ReactNode} from 'react';
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
  if (session.ended) return <StatusDot kind="ended" title="Ended" />;
  if (waiting) return <StatusDot kind="waiting" title="Waiting on you" />;
  if (session.unseen && !selected) return <StatusDot kind="unseen" title="New reply" />;
  if (session.activity === 'busy') return <BusyDot title="Working" />;
  return <StatusDot kind="idle" title="Idle" />;
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
  const classes = `session-item group/session relative h-7 cursor-default justify-start px-2 font-normal ${selected ? 'hover:bg-secondary' : ''} ${session.ended ? 'opacity-60' : ''}`;
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
      <span className="session-item-status inline-flex size-4 shrink-0 items-center justify-center">{status}</span>
      <span className="flex min-w-0 flex-1 items-center">
        {session.id === renaming
          ? <RenameField session={session} onDone={() => onRenaming(null)} />
          : <Title title={session.title} small={level === 2} chevron={hasChildren ? <Chevron open={expanded} /> : null} onToggle={toggle} />}
      </span>
      {session.id !== renaming && (session.worktree || parent) && <span className="session-item-decoration flex shrink-0 items-center gap-1">
        {parent && <SettingsDiffer child={session} parent={parent} workspace={sessionOf(state, session.id)?.workspace.id ?? null} />}
        {session.worktree && <BranchIcon className="session-item-branch" merge={session.merge} />}
      </span>}
    </div></Button>
  );
}

// A title too long to fit fades out over 1rem ending --fade-end before its
// right edge.
const fade = '[mask-image:linear-gradient(to_right,#000_calc(100%_-_var(--fade-end)_-_1rem),transparent_calc(100%_-_var(--fade-end)))] [--fade-end:0px]';
// While a parent's chevron shows, the title's padding makes room for it, and
// the fade ends before it.
const chevronRoom = 'group-hover/session:pr-5 group-focus-visible/session:pr-5';
const fadeBeforeChevron = 'group-hover/session:[--fade-end:1.25rem] group-focus-visible/session:[--fade-end:1.25rem]';

interface TitleProps {
  title: string;
  small: boolean;
  chevron: ReactNode;
  onToggle: (e: MouseEvent) => void;
}

// Title shows a session title and, for a parent, the chevron at its end. It
// watches whether the title overflows, so only a title that does not fit
// fades.
function Title({title, small, chevron, onToggle}: TitleProps) {
  const ref = useRef<HTMLSpanElement>(null);
  const [overflows, setOverflows] = useState(false);

  useLayoutEffect(() => {
    const element = ref.current;
    if (!element) return;
    const measure = () => setOverflows(element.scrollWidth > element.clientWidth);
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(element);
    return () => observer.disconnect();
  }, [title]);

  return (
    <span className="relative flex min-w-0 items-center">
      <span ref={ref} className={`session-item-title min-w-0 overflow-hidden whitespace-nowrap text-left ${small ? 'text-xs' : ''} ${chevron ? chevronRoom : ''} ${overflows ? fade : ''} ${overflows && chevron ? fadeBeforeChevron : ''}`}>{title}</span>
      {chevron && <span className="session-item-twistie invisible absolute right-0 top-1/2 flex size-4 -translate-y-1/2 items-center group-hover/session:visible group-focus-visible/session:visible" onClick={onToggle}>
        {chevron}
      </span>}
    </span>
  );
}

// Branch indents a child row and draws its stretch of the line down from the
// center of the parent's status dot.
function Branch({position, siblings}: {position: number; siblings: number}) {
  return (
    <span aria-hidden="true" className="pointer-events-none relative w-3 shrink-0 self-stretch">
      <span className={`absolute left-2 border-l border-border ${position === 1 ? 'top-1' : 'top-0'} ${position === siblings ? 'bottom-1/2 w-1 rounded-bl-sm border-b' : 'bottom-0'}`} />
    </span>
  );
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
    <Button asChild variant="ghost" size="sm" className="session-fold h-7 cursor-default justify-start px-2 text-xs font-normal text-muted-foreground"><div
      role="treeitem" data-row={id} aria-level={2} aria-posinset={position} aria-setsize={siblings} aria-expanded={expanded} aria-selected={false}
      tabIndex={tabbable ? 0 : -1} onFocus={onFocus} onClick={onToggle}
    >
      <Branch position={position} siblings={siblings} />
      <span className="ml-1 flex items-center gap-1">{count} ended<Chevron open={expanded} /></span>
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
