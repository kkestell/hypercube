// A session's transcript in a scrolling list. The list follows its end until
// the user scrolls up, and follows again once they scroll back to the end.
// Growing content leaves scrollTop alone, so only a scroll upward stops
// following; the end allows for fractional scroll positions. The footer sits
// below the scrolling view, so the scrollbar stops above it.
// The list, the pending card, and the composer share one padded column.
// Expanding or collapsing an item keeps its heading where it was on screen,
// and the list follows its end again only if that leaves it there.
import {Fragment, useLayoutEffect, useMemo, useRef, type MouseEvent, type ReactNode} from 'react';
import {focusComposerField} from '../agent/focus-composer.ts';
import {isUnsent} from '../agent/drafts.ts';
import {Button} from '../components/ui/button';
import {Card, CardContent} from '../components/ui/card';
import {IconButton} from '../shared/icon-button.tsx';
import {useScrollMemory} from '../editor/use-scroll-memory.ts';
import {useStore} from '../store.ts';
import {Find} from '../shared/find.tsx';
import {tabBackground} from '../shared/tab-background.ts';
import type {Session} from '../sessions/types.ts';
import {dismissSubmission, retrySubmission} from '../sessions/session-actions.ts';
import {submissionsOf} from '../sessions/submissions.ts';
import {tones} from '../shared/tones.ts';
import {AttentionItem} from './attention-item.tsx';
import {QuestionsForm} from './attention-form.tsx';
import {ProposalItem} from './proposal-item.tsx';
import {TranscriptRow} from './transcript-item.tsx';
import {isPending, isQuestion, pendingGroups, transcriptRows} from './transcript-tree.ts';
import {displayedTranscript} from './transcripts.ts';
import type {AttentionItem as Attention} from './types.ts';

interface TranscriptProps {
  session: Pick<Session, 'id' | 'ended'>;
  active?: boolean;
  footer?: ReactNode;
}

export function Transcript({session, active = true, footer}: TranscriptProps) {
  const [state, store] = useStore();
  const transcript = useMemo(() => displayedTranscript(state, session.id), [state.session?.transcript, state.submissions, session.id]);
  const submissions = submissionsOf(state, session.id);
  const ready = isUnsent(session.id) || (state.session?.transcriptReady ?? false);
  const rows = useMemo(() => transcriptRows(transcript), [transcript]);
  const pending = transcript.order.map(id => transcript.items[id]).filter(isPending);

  const view = useRef<HTMLDivElement>(null), list = useRef<HTMLElement>(null);
  const {away, followEnd, holdPosition} = useScrollMemory(view, session.id, ready, true);
  const held = useRef<{trigger: Element; top: number} | null>(null);

  // Every expand and collapse trigger in the list toggles through the store,
  // so the render that follows a click on one finds it held.
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
      holdPosition(hold.trigger.getBoundingClientRect().top - hold.top);
    } else followEnd();
  });
  useLayoutEffect(() => {
    if (!view.current || !list.current) return;
    const observer = new ResizeObserver(() => followEnd());
    observer.observe(view.current);
    observer.observe(list.current);
    return () => observer.disconnect();
  }, [ready]);

  return (
    <div className="relative flex min-h-0 flex-1 flex-col text-sm">
      <Find list={list} view={view} active={active} label="Find in transcript" className="transcript-find flex shrink-0 items-center gap-2 border-b p-2" />
      <div ref={view} className="transcript flex min-h-0 flex-1 flex-col overflow-auto">
        <section ref={list} aria-label="Transcript" onClickCapture={holdTrigger} className="transcript-list flex w-full flex-1 flex-col gap-3 px-4 py-4">
          {rows.map(row => {
            const id = row.type === 'activity' ? row.key : row.item.id;
            const submission = submissions.find(entry => entry.id === id);
            return <Fragment key={id}>
              <TranscriptRow row={row} />
              {submission?.status === 'sending' && <div role="status" className="text-right text-xs text-muted-foreground">Sending…</div>}
              {submission?.status === 'failed' && <div className="flex flex-col items-end gap-2">
                <div role="alert" className={`text-right text-xs ${tones.error}`}>Sending failed: {submission.error}</div>
                <div className="flex gap-2">
                  <Button size="sm" variant="secondary" disabled={session.ended} onClick={() => retrySubmission(store, session.id, id)}>Retry</Button>
                  <Button size="sm" variant="ghost" onClick={() => dismissSubmission(store, session.id, id)}>Dismiss</Button>
                </div>
              </div>}
            </Fragment>;
          })}
        </section>
      </div>
      <div className={`transcript-footer relative flex max-h-3/4 min-h-0 shrink-0 flex-col ${tabBackground}`}>
        {away && (
          <IconButton icon="arrow-down" label="Scroll to newest output" type="button" variant="secondary" className="absolute -top-9 left-1/2 -translate-x-1/2 rounded-full animate-in fade-in"
            onClick={() => { followEnd(true); focusComposerField(); }} />
        )}
        <div className="flex min-h-0 flex-col overflow-y-auto">
          {/* Pending content scrolls above the composer. If even one control
              and the composer cannot fit, their shared container also scrolls.
              The padded card bottom tucks 16px under the composer. */}
          {pending.length > 0 && (
            <section className="-mb-8 min-h-24 shrink overflow-y-auto px-4" aria-label="Waiting on you">
              <Card className="mx-3 gap-4 rounded-b-none rounded-t-2xl border-b-0 pb-8 pt-4 shadow-none">
                {/* A group of questions keeps its place while its answers
                    are sent and its questions close one by one. */}
                {pendingGroups(pending).map((group, i) => isQuestion(group[0])
                  ? <CardContent key={`questions-${i}`} className="px-4"><QuestionsForm session={session.id} items={group as Attention[]} ended={session.ended} /></CardContent>
                  : <CardContent key={group[0].id} className="px-4">{group[0].kind === 'attention'
                    ? <AttentionItem item={group[0]} session={session} />
                    : <ProposalItem item={group[0]} session={session} />}</CardContent>)}
              </Card>
            </section>
          )}
          <div className={`relative shrink-0 ${pending.length ? 'drop-shadow-lg dark:drop-shadow-black/40' : ''}`}>{footer}</div>
        </div>
      </div>
    </div>
  );
}
