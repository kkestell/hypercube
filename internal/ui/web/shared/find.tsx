// Find and navigate matches in rendered content as it changes. The find bar
// fades in over the top right corner of its view, or, inline, as a row above
// it.
import {useEffect, useLayoutEffect, useRef, useState, type RefObject} from 'react';
import {Input} from '../components/ui/input';
import {matches} from '../app/shortcuts.ts';
import {findMatches} from './find-matches.ts';
import {focusedElement} from './focus.ts';
import {IconButton} from './icon-button.tsx';

interface FindProps {
  list: RefObject<HTMLElement | null>;
  view: RefObject<HTMLElement | null>;
  active: boolean;
  label: string;
  inline?: boolean;
}

export function Find({list, view, active, label, inline = false}: FindProps) {
  const [open, setOpen] = useState(false), [query, setQuery] = useState('');
  const [ranges, setRanges] = useState<Range[]>([]), [index, setIndex] = useState(0);
  const input = useRef<HTMLInputElement>(null), previousFocus = useRef<HTMLElement | null>(null);

  const step = (delta: number) => { if (ranges.length) setIndex(i => (i + delta + ranges.length) % ranges.length); };
  const selectInput = () => {
    const field = input.current;
    field?.focus();
    field?.select();
  };
  useEffect(() => {
    const keydown = (event: KeyboardEvent) => {
      if (!active || event.defaultPrevented) return;
      if (matches(event, 'find')) {
        event.preventDefault();
        if (!open) previousFocus.current = focusedElement();
        setOpen(true);
        if (open) selectInput();
      } else if (open && (matches(event, 'findNext') || matches(event, 'findPrevious'))) {
        event.preventDefault();
        step(matches(event, 'findPrevious') ? -1 : 1);
      }
    };
    document.addEventListener('keydown', keydown);
    return () => document.removeEventListener('keydown', keydown);
  }, [active, open, ranges]);

  useLayoutEffect(() => { if (open) selectInput(); }, [open]);
  useLayoutEffect(() => {
    setIndex(0);
    if (!open) { setRanges([]); return; }
    const root = list.current;
    if (!root) return;
    const update = () => {
      const found = findMatches(root, query);
      setRanges(found);
      setIndex(i => Math.min(i, Math.max(0, found.length - 1)));
    };
    update();
    const observer = new MutationObserver(update);
    observer.observe(root, {childList: true, characterData: true, subtree: true});
    return () => observer.disconnect();
  }, [open, query]);
  // A highlight marks the match without moving the selection, which would take
  // focus from the find field in WebKit.
  useLayoutEffect(() => {
    if (!active || !open || !ranges[index]) return;
    const highlight = new Highlight(ranges[index]);
    CSS.highlights.set('find', highlight);
    return () => { if (CSS.highlights.get('find') === highlight) CSS.highlights.delete('find'); };
  }, [active, open, ranges, index]);
  useLayoutEffect(() => {
    const range = ranges[index], scroller = view.current;
    if (!range || !scroller) return;
    const rect = range.getBoundingClientRect();
    const bounds = scroller.getBoundingClientRect();
    if (rect.top < bounds.top || rect.bottom > bounds.bottom) scroller.scrollTop += rect.top - bounds.top - bounds.height / 2;
    if (rect.left < bounds.left) scroller.scrollLeft += rect.left - bounds.left;
    else if (rect.right > bounds.right) scroller.scrollLeft += rect.right - bounds.right;
  }, [ranges, index]);

  const close = () => {
    setOpen(false);
    previousFocus.current?.focus();
  };
  if (!open) return null;
  const place = inline ? 'shrink-0 border-b' : 'absolute right-2 top-2 z-10 rounded-md border bg-background shadow-md';
  return <div className={`flex animate-in fade-in items-center gap-2 p-2 ${place}`} role="search" aria-label={label} onKeyDown={event => {
    if (event.key === 'Escape') { event.preventDefault(); close(); }
  }}>
    <Input ref={input} value={query} placeholder="Find" aria-label={label} onChange={event => setQuery(event.currentTarget.value)} onKeyDown={event => {
      if (event.key === 'Enter') { event.preventDefault(); step(event.shiftKey ? -1 : 1); }
    }} />
    <span className="shrink-0 text-xs text-muted-foreground" aria-live="polite">{ranges.length ? `${index + 1} of ${ranges.length}` : 'No results'}</span>
    <IconButton icon="arrow-up" label="Previous match" shortcut="findPrevious" type="button" onClick={() => step(-1)} disabled={!ranges.length} />
    <IconButton icon="arrow-down" label="Next match" shortcut="findNext" type="button" onClick={() => step(1)} disabled={!ranges.length} />
    <IconButton icon="close" label="Close find" type="button" onClick={close} />
  </div>;
}
