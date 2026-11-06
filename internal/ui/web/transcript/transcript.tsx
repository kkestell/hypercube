// A session's transcript in a scrolling list. The list follows its end until
// the user scrolls up, and follows again once they scroll back to the end.
// Growing content leaves scrollTop alone, so only a scroll upward stops
// following; the end allows for fractional scroll positions. The footer sits
// below the scrolling view, so the scrollbar stops above it.
// The list's sides line up with the composer's attach and send icons: the
// column's 16px plus the composer box's 21px to its icons. Expanding or
// collapsing an item keeps its heading where it was on screen, and the list
// follows its end again only if that leaves it there.
import {useCallback, useLayoutEffect, useMemo, useRef, useState, type MouseEvent, type ReactNode} from 'react';
import {column} from '../app/workbench.ts';
import {focusComposerField} from '../agent/focus-composer.ts';
import {Button} from '../components/ui/button';
import {Card, CardContent} from '../components/ui/card';
import {Icon} from '../shared/icon.tsx';
import {useScrollMemory} from '../editor/use-scroll-memory.ts';
import {rememberScroll} from '../editor/editor.ts';
import {useStore} from '../store.ts';
import {Find} from '../shared/find.tsx';
import {tabBackground} from '../shared/tab-background.ts';
import type {Session} from '../sessions/types.ts';
import {AttentionItem} from './attention-item.tsx';
import {ProposalItem} from './proposal-item.tsx';
import {TranscriptRow} from './transcript-item.tsx';
import {isPending, transcriptRows} from './transcript-tree.ts';
import {emptyTranscript, setExpanded} from './transcripts.ts';

interface TranscriptProps {
  session: Pick<Session, 'id' | 'ended'>;
  active?: boolean;
  footer?: ReactNode;
}

export function Transcript({session, active = true, footer}: TranscriptProps) {
  const [state, store] = useStore();
  const transcript = state.session?.transcript ?? emptyTranscript;
  const ready = state.session?.transcriptReady ?? false;
  const expanded = state.session?.expanded ?? {};
  const rows = useMemo(() => transcriptRows(transcript), [transcript]);
  const pending = transcript.order.map(id => transcript.items[id]).filter(isPending);
  const setOpen = useCallback((key: string, open: boolean) => store.update(s => setExpanded(s, key, open)), [store]);

  const view = useRef<HTMLDivElement>(null), list = useRef<HTMLDivElement>(null);
  const memory = useScrollMemory(view, session.id, ready);
  const lastTop = useRef(memory.current.top);
  const [away, setAway] = useState(!memory.current.following);
  const held = useRef<{trigger: Element; top: number} | null>(null);

  const followEnd = () => {
    const container = view.current;
    if (!ready || !container || !memory.current.following) return;
    container.scrollTop = container.scrollHeight;
    lastTop.current = container.scrollTop;
    setAway(false);
  };

  const scrolled = () => {
    if (!ready || !view.current) return;
    const {scrollTop, clientHeight, scrollHeight} = view.current;
    setAway(scrollHeight - scrollTop - clientHeight >= 2);
    let following = memory.current.following;
    if (scrollHeight - scrollTop - clientHeight < 2) following = true;
    else if (scrollTop < lastTop.current) following = false;
    lastTop.current = scrollTop;
    memory.current = {...memory.current, top: scrollTop, left: view.current.scrollLeft, following};
    store.update(state => rememberScroll(state, session.id, session.id, memory.current));
  };

  // Every expand and collapse trigger in the list toggles through setOpen, so
  // the render that follows a click on one finds it held.
  const holdTrigger = (e: MouseEvent) => {
    const trigger = e.target instanceof Element && e.target.closest('[data-slot=collapsible-trigger]');
    if (trigger) held.current = {trigger, top: trigger.getBoundingClientRect().top};
  };

  // New items, a resized pane, and a view shown again keep a following list
  // at its end. A toggled item instead keeps its trigger in place.
  useLayoutEffect(() => {
    const hold = held.current;
    held.current = null;
    if (hold && view.current) {
      view.current.scrollTop += hold.trigger.getBoundingClientRect().top - hold.top;
      memory.current = {...memory.current, following: false};
    } else followEnd();
    scrolled();
  });
  useLayoutEffect(() => {
    if (!view.current || !list.current) return;
    const observer = new ResizeObserver(followEnd);
    observer.observe(view.current);
    observer.observe(list.current);
    return () => observer.disconnect();
  }, [ready]);

  return (
    <div className="transcript-container relative flex min-h-0 flex-1 flex-col text-sm">
      <Find list={list} view={view} active={active} label="Find in transcript" className="transcript-find flex shrink-0 items-center gap-2 border-b p-2" />
      <div ref={view} className="transcript flex min-h-0 flex-1 flex-col overflow-auto" onScroll={scrolled}>
        <div ref={list} onClickCapture={holdTrigger} className={`transcript-list mx-auto flex w-full flex-1 flex-col gap-3 px-[37px] py-4 ${state.workbench.fixedWidth ? 'max-w-[50.6rem]' : ''}`}>
          {rows.map(row => <TranscriptRow key={row.type === 'activity' ? row.key : row.item.id} row={row} session={session} expanded={expanded} setOpen={setOpen} />)}
        </div>
      </div>
      <div className={`transcript-footer relative shrink-0 ${tabBackground} ${pending.length ? '[&_.composer>[data-slot=card]]:shadow-lg dark:[&_.composer>[data-slot=card]]:shadow-black/40' : ''}`}>
        {away && (
          <Button type="button" variant="secondary" size="icon" className="absolute -top-12 left-1/2 -translate-x-1/2 rounded-full" title="Scroll to newest output" aria-label="Scroll to newest output"
            onClick={() => { memory.current = {...memory.current, following: true}; followEnd(); focusComposerField(); }}><Icon name="arrow-down" /></Button>
        )}
        {/* The card tucks 16px under the composer, which comes after it in tab
            order, overlaps its padded bottom edge, and casts a shadow on it. */}
        {pending.length > 0 && (
          <section className={`transcript-pending -mb-8 ${column(state)}`} aria-label="Waiting on you">
            <Card className="mx-3 gap-4 rounded-b-none rounded-t-[16px] border-b-0 pb-8 pt-4 shadow-none">
              {pending.map(item => <CardContent key={item.id} className="px-4">{item.kind === 'attention'
                ? <AttentionItem item={item} session={session} />
                : <ProposalItem item={item} session={session} expanded={false} setOpen={setOpen} />}</CardContent>)}
            </Card>
          </section>
        )}
        {footer}
      </div>
    </div>
  );
}
