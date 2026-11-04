// A session's transcript in a scrolling list. The list follows its end until
// the user scrolls up, and follows again once they scroll back to the end.
// Growing content leaves scrollTop alone, so only a scroll upward stops
// following; the end allows for fractional scroll positions.
import {useCallback, useLayoutEffect, useMemo, useRef} from 'react';
import {useScrollMemory} from '../editor/use-scroll-memory.js';
import {useStore} from '../store.js';
import {TranscriptFind} from './transcript-find.jsx';
import {AttentionItem} from './attention-item.jsx';
import {TranscriptRow} from './transcript-item.jsx';
import {transcriptRows} from './transcript-tree.js';
import {emptyTranscript, setExpanded} from './transcripts.js';

export function Transcript({session}) {
  const [state, store] = useStore();
  const transcript = state.session?.transcript ?? emptyTranscript;
  const expanded = state.session?.expanded ?? {};
  const rows = useMemo(() => transcriptRows(transcript), [transcript]);
  const pending = transcript.order.map(id => transcript.items[id]).filter(item => item.kind === 'attention' && !item.done);
  const setOpen = useCallback((key, open) => store.update(s => setExpanded(s, key, open)), [store]);

  const view = useRef(null), list = useRef(null);
  const memory = useScrollMemory(view, session.id);
  const lastTop = useRef(memory.current.top);

  const followEnd = () => {
    if (!memory.current.following) return;
    view.current.scrollTop = view.current.scrollHeight;
    lastTop.current = view.current.scrollTop;
  };

  const scrolled = () => {
    const {scrollTop, clientHeight, scrollHeight} = view.current;
    let following = memory.current.following;
    if (scrollHeight - scrollTop - clientHeight < 2) following = true;
    else if (scrollTop < lastTop.current) following = false;
    lastTop.current = scrollTop;
    memory.current = {...memory.current, following};
  };

  // New items, opened details, a resized pane, and a view shown again all
  // keep a following list at its end.
  useLayoutEffect(followEnd);
  useLayoutEffect(() => {
    const observer = new ResizeObserver(followEnd);
    observer.observe(view.current);
    observer.observe(list.current);
    return () => observer.disconnect();
  }, []);

  return (
    <div className="transcript-container relative flex min-h-0 flex-1 flex-col">
      <TranscriptFind list={list} view={view} />
      <div ref={view} className="transcript min-h-0 flex-1 overflow-auto" onScroll={scrolled}>
        <div ref={list} className="transcript-list flex flex-col gap-4 p-4">
          {rows.map(row => <TranscriptRow key={row.key ?? row.item.id} row={row} session={session} expanded={expanded} setOpen={setOpen} />)}
        </div>
      </div>
      {pending.length > 0 && (
        <section className="transcript-pending flex max-h-1/2 shrink-0 flex-col gap-4 overflow-y-auto border-t p-4" aria-label="Pending questions">
          {pending.map(item => <AttentionItem key={item.id} item={item} session={session} />)}
        </section>
      )}
    </div>
  );
}
