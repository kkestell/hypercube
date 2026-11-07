// A session's tree row. A parent shows a chevron after its title on hover or
// keyboard focus. Before the title, a fixed slot shows the session's state as a
// dot, hollow for a child. The row's right edge shows the worktree branch icon.
// A click chooses the session; a click on a parent's chevron, or a double-click
// on the parent, collapses or expands its children.
import {useEffect, useLayoutEffect, useRef, useState, type KeyboardEvent, type MouseEvent, type ReactNode} from 'react';
import {chooseSession, renameSession} from '../sessions/session-actions.ts';
import {shownId} from '../sessions/selection.ts';
import type {Session} from '../sessions/types.ts';
import {BranchIcon} from '../shared/branch-icon.tsx';
import {StatusDot} from '../shared/status-dot.tsx';
import {useStore} from '../store.ts';
import {Button} from '../components/ui/button';
import {Input} from '../components/ui/input';
import {BusyDot} from '../shared/busy-dot.tsx';
import {Chevron} from '../shared/icon.tsx';

// statusOf returns a session's status dot, hollow when outline is set.
const statusOf = (session: Session, selected: boolean, waiting: boolean, outline: boolean) => {
  if (session.ended) return <StatusDot kind="ended" title="Ended" />;
  if (waiting) return <StatusDot kind="waiting" title="Waiting on you" outline={outline} />;
  if (session.unseen && !selected) return <StatusDot kind="unseen" title="New reply" outline={outline} />;
  if (session.activity === 'busy') return <BusyDot title="Working" outline={outline} />;
  return <StatusDot kind="idle" title="Idle" outline={outline} />;
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
  const selected = session.id === shownId(state);
  const status = statusOf(session, selected, session.attention || state.requests.some(request => request.session === session.id), level === 2);
  const classes = `session-item group/session relative h-7 cursor-default justify-start px-2 font-normal ${selected ? 'hover:bg-secondary' : ''}`;
  // An ended session dims its contents but not its guide line, which stays
  // even along the tree.
  const dim = session.ended ? 'opacity-60' : '';
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
      <span className={`session-item-status inline-flex size-4 shrink-0 items-center justify-center ${dim}`}>{status}</span>
      <span className={`flex min-w-0 flex-1 items-center ${dim} ${session.worktree ? '' : `${toEdge} ${hasChildren ? chevronInset : ''}`}`}>
        {session.id === renaming
          ? <RenameField session={session} onDone={() => onRenaming(null)} />
          : <Title title={session.title} small={level === 2} chevron={hasChildren ? <Chevron open={expanded} /> : null} onToggle={toggle} />}
      </span>
      {session.id !== renaming && session.worktree && <BranchIcon className={`session-item-branch shrink-0 ${dim}`} merge={session.merge} />}
    </div></Button>
  );
}

// A title too long to fit fades out over --fade-width, 1.5rem before an icon,
// ending --fade-end before its right edge.
const fade = '[mask-image:linear-gradient(to_right,#000_calc(100%_-_var(--fade-end)_-_var(--fade-width,1.5rem)),transparent_calc(100%_-_var(--fade-end)))] [--fade-end:0px]';
// With no icon after it, a title runs through the row's padding to its edge
// and fades out over a shorter 1rem. While a parent's chevron shows, the
// title stays inside the padding, so the chevron does too.
const toEdge = '-mr-2 [--fade-width:1rem]';
const chevronInset = 'group-hover/session:mr-0 group-focus-visible/session:mr-0';
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

// Branch indents a child row and draws its stretch of the line that runs down
// from just under the parent's status dot, centered on it, with a stroke across
// toward the child's dot that starts beside the line, so the translucent
// strokes never overlap. The last child's stroke turns the line's corner.
function Branch({position, siblings}: {position: number; siblings: number}) {
  const last = position === siblings;
  return (
    <span aria-hidden="true" className="pointer-events-none relative w-3.5 shrink-0 self-stretch">
      <span className={`absolute left-[7.5px] border-l border-border ${position === 1 ? '-top-1' : 'top-0'} ${last ? 'bottom-1/2 w-3 rounded-bl-sm border-b' : 'bottom-0'}`} />
      {!last && <span className="absolute bottom-1/2 left-[8.5px] w-[11px] border-b border-border" />}
    </span>
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
