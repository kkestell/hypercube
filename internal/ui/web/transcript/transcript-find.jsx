import {Button} from '../components/ui/button';
import {Input} from '../components/ui/input';
import {Icon} from '../shared/icon.jsx';
// The transcript's find widget follows rendered content as it changes.
import {useEffect, useLayoutEffect, useRef, useState} from 'react';
import {transcriptMatches} from './transcript-matches.js';

export function TranscriptFind({list, view}) {
  const [open, setOpen] = useState(false), [query, setQuery] = useState('');
  const [ranges, setRanges] = useState([]), [index, setIndex] = useState(0);
  const input = useRef(null), previousFocus = useRef(null);

  const step = delta => { if (ranges.length) setIndex(i => (i + delta + ranges.length) % ranges.length); };
  const selectInput = () => {
    const field = input.current;
    field?.focus();
    field?.select();
  };
  useEffect(() => {
    const keydown = event => {
      const focus = document.activeElement;
      const pane = view.current.closest('.agent-pane');
      if (focus !== document.body && focus !== document.documentElement && !(pane ?? view.current.parentElement).contains(focus)) return;
      if (event.altKey || event.ctrlKey) return;
      const key = event.key.toLowerCase();
      if (key === 'f' && event.metaKey && !event.shiftKey) {
        event.preventDefault();
        if (!open) previousFocus.current = focus;
        setOpen(true);
        if (open) selectInput();
      } else if (open && ((key === 'g' && event.metaKey) || (key === 'f3' && !event.metaKey))) {
        event.preventDefault();
        step(event.shiftKey ? -1 : 1);
      }
    };
    document.addEventListener('keydown', keydown);
    return () => document.removeEventListener('keydown', keydown);
  }, [open, ranges]);

  useLayoutEffect(() => { if (open) selectInput(); }, [open]);
  useLayoutEffect(() => {
    setIndex(0);
    if (!open) { setRanges([]); return; }
    const update = () => {
      const found = transcriptMatches(list.current, query);
      setRanges(found);
      setIndex(i => Math.min(i, Math.max(0, found.length - 1)));
    };
    update();
    const observer = new MutationObserver(update);
    observer.observe(list.current, {childList: true, characterData: true, subtree: true});
    return () => observer.disconnect();
  }, [open, query]);
  useLayoutEffect(() => {
    const selection = window.getSelection();
    if (!ranges[index]) return;
    selection.removeAllRanges();
    selection.addRange(ranges[index]);
    return () => selection.removeAllRanges();
  }, [ranges, index]);
  useLayoutEffect(() => {
    const range = ranges[index];
    if (!range) return;
    const rect = range.getBoundingClientRect();
    const bounds = view.current.getBoundingClientRect();
    if (rect.top < bounds.top || rect.bottom > bounds.bottom) view.current.scrollTop += rect.top - bounds.top - bounds.height / 2;
  }, [ranges, index]);

  const close = () => {
    setOpen(false);
    previousFocus.current?.focus();
  };
  if (!open) return null;
  return <div className="transcript-find flex shrink-0 items-center gap-2 border-b p-2" role="search" aria-label="Find in transcript" onKeyDown={event => {
    if (event.key === 'Escape') { event.preventDefault(); close(); }
  }}>
    <Input ref={input} value={query} placeholder="Find" aria-label="Find in transcript" aria-invalid={!!query && !ranges.length} onChange={event => setQuery(event.currentTarget.value)} onKeyDown={event => {
      if (event.key === 'Enter') { event.preventDefault(); step(event.shiftKey ? -1 : 1); }
    }} />
    <span className="transcript-find-count shrink-0 text-xs text-muted-foreground" aria-live="polite">{ranges.length ? `${index + 1} of ${ranges.length}` : 'No results'}</span>
    <Button variant="ghost" size="icon" type="button" aria-label="Previous match" title="Previous match" onClick={() => step(-1)} disabled={!ranges.length}><Icon name="arrow-up" /></Button>
    <Button variant="ghost" size="icon" type="button" aria-label="Next match" title="Next match" onClick={() => step(1)} disabled={!ranges.length}><Icon name="arrow-down" /></Button>
    <Button variant="ghost" size="icon" type="button" aria-label="Close find" title="Close" onClick={close}><Icon name="close" /></Button>
  </div>;
}
