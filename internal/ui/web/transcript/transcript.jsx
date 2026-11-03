// A session's transcript in a scrolling list. The list follows its end until
// the user scrolls up, and follows again once they scroll back to the end.
// Growing content leaves scrollTop alone, so only a scroll upward stops
// following; the end allows for fractional scroll positions.
import {useCallback, useLayoutEffect, useMemo, useRef} from 'preact/hooks';
import {useScrollMemory} from '../editor/use-scroll-memory.js';
import {useStore} from '../store.js';
import {ClockProvider} from './elapsed.jsx';
import {TranscriptRow} from './transcript-item.jsx';
import {transcriptRows} from './transcript-tree.js';
import {emptyTranscript, setExpanded} from './transcripts.js';
import './transcript.css';

export function Transcript({session}) {
  const [state, store] = useStore();
  const transcript = state.session?.transcript ?? emptyTranscript;
  const expanded = state.session?.expanded ?? {};
  const rows = useMemo(() => transcriptRows(transcript), [transcript]);
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
    <div ref={view} class="transcript" onScroll={scrolled}>
      <div ref={list} class="transcript-list">
        <ClockProvider>
          {rows.map(row => <TranscriptRow key={row.key ?? row.item.id} row={row} session={session} expanded={expanded} setOpen={setOpen} />)}
        </ClockProvider>
      </div>
    </div>
  );
}
