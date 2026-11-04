// The transcript's find widget follows rendered content as it changes.
import {useEffect, useLayoutEffect, useRef, useState} from 'preact/hooks';
import {transcriptMatches} from './transcript-matches.js';
import './transcript-find.css';

export function TranscriptFind({list, view}) {
  const [open, setOpen] = useState(false), [query, setQuery] = useState('');
  const [ranges, setRanges] = useState([]), [index, setIndex] = useState(0);
  const input = useRef(null), previousFocus = useRef(null);

  const step = delta => { if (ranges.length) setIndex(i => (i + delta + ranges.length) % ranges.length); };
  const selectInput = () => {
    const field = input.current;
    field?.focus();
    field?.updateComplete?.then(() => field.wrappedElement.select());
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
    if (!globalThis.CSS?.highlights || !globalThis.Highlight) return;
    CSS.highlights.set('transcript-find', new Highlight(...ranges));
    CSS.highlights.set('transcript-find-current', new Highlight(...(ranges[index] ? [ranges[index]] : [])));
    return () => {
      CSS.highlights.delete('transcript-find');
      CSS.highlights.delete('transcript-find-current');
    };
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
  return <div class="transcript-find" role="search" aria-label="Find in transcript" onKeyDown={event => {
    if (event.key === 'Escape') { event.preventDefault(); close(); }
  }}>
    <vscode-textfield ref={input} value={query} placeholder="Find" label="Find in transcript" class={query && !ranges.length ? 'transcript-find-empty' : undefined} onInput={event => setQuery(event.currentTarget.value)} onKeyDown={event => {
      if (event.key === 'Enter') { event.preventDefault(); step(event.shiftKey ? -1 : 1); }
    }} />
    <span class="transcript-find-count" aria-live="polite">{ranges.length ? `${index + 1} of ${ranges.length}` : 'No results'}</span>
    <button type="button" aria-label="Previous match" title="Previous match" onClick={() => step(-1)} disabled={!ranges.length}><vscode-icon name="arrow-up" /></button>
    <button type="button" aria-label="Next match" title="Next match" onClick={() => step(1)} disabled={!ranges.length}><vscode-icon name="arrow-down" /></button>
    <button type="button" aria-label="Close find" title="Close" onClick={close}><vscode-icon name="close" /></button>
  </div>;
}
