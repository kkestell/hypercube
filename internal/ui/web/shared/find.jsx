import {Button} from '../components/ui/button';
import {Input} from '../components/ui/input';
import {Icon} from './icon.jsx';
import {useEffect, useLayoutEffect, useRef, useState} from 'react';
import {matches} from '../app/shortcuts.js';
import {findMatches} from './find-matches.js';

// Find and navigate matches in rendered content as it changes.
export function Find({list, view, active, label, className}) {
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
      if (!active || event.defaultPrevented) return;
      const focus = document.activeElement;
      if (matches(event, 'find')) {
        event.preventDefault();
        if (!open) previousFocus.current = focus;
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
    const update = () => {
      const found = findMatches(list.current, query);
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
    if (rect.left < bounds.left) view.current.scrollLeft += rect.left - bounds.left;
    else if (rect.right > bounds.right) view.current.scrollLeft += rect.right - bounds.right;
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
