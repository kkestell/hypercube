import {Button} from '../components/ui/button';
import {Input} from '../components/ui/input';
import {Icon} from './icon.tsx';
import {useEffect, useLayoutEffect, useRef, useState, type RefObject} from 'react';
import {matches} from '../app/shortcuts.ts';
import {findMatches} from './find-matches.ts';
import {focusedElement} from './focus.ts';

// Find and navigate matches in rendered content as it changes.
interface FindProps {
  list: RefObject<HTMLElement | null>;
  view: RefObject<HTMLElement | null>;
  active: boolean;
  label: string;
  className?: string;
}

export function Find({list, view, active, label, className}: FindProps) {
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
    if (!ranges[index]) return;
    CSS.highlights.set('find', new Highlight(ranges[index]));
    return () => { CSS.highlights.delete('find'); };
  }, [ranges, index]);
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
  return <div className={className} role="search" aria-label={label} onKeyDown={event => {
    if (event.key === 'Escape') { event.preventDefault(); close(); }
  }}>
    <Input ref={input} value={query} placeholder="Find" aria-label={label} onChange={event => setQuery(event.currentTarget.value)} onKeyDown={event => {
      if (event.key === 'Enter') { event.preventDefault(); step(event.shiftKey ? -1 : 1); }
    }} />
    <span className="transcript-find-count shrink-0 text-xs text-muted-foreground" aria-live="polite">{ranges.length ? `${index + 1} of ${ranges.length}` : 'No results'}</span>
    <Button variant="ghost" size="icon" type="button" aria-label="Previous match" title="Previous match" onClick={() => step(-1)} disabled={!ranges.length}><Icon name="arrow-up" /></Button>
    <Button variant="ghost" size="icon" type="button" aria-label="Next match" title="Next match" onClick={() => step(1)} disabled={!ranges.length}><Icon name="arrow-down" /></Button>
    <Button variant="ghost" size="icon" type="button" aria-label="Close find" title="Close" onClick={close}><Icon name="close" /></Button>
  </div>;
}
