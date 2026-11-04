import {useEffect, useLayoutEffect, useRef, useState} from 'preact/hooks';
import './transcript-find.css';

// Find across inline markup without changing the rendered transcript.
function matchesIn(list, query) {
  if (!query) return [];
  const walker = document.createTreeWalker(list, NodeFilter.SHOW_TEXT, {
    acceptNode: node => node.parentElement.closest('button') ? NodeFilter.FILTER_REJECT : NodeFilter.FILTER_ACCEPT,
  });
  const nodes = [];
  let text = '', node, block;
  while ((node = walker.nextNode())) {
    const current = node.parentElement.closest('p, pre, h1, h2, h3, h4, h5, h6, li, td, th, summary, .message-item-text, .transcript-error');
    if (text && current !== block) text += '\n';
    block = current;
    nodes.push({node, start: text.length});
    text += node.data;
  }
  const ranges = [], needle = query.toLowerCase(), haystack = text.toLowerCase();
  for (let start = 0; (start = haystack.indexOf(needle, start)) !== -1; start += needle.length) {
    const end = start + needle.length;
    const first = nodes.findLast(n => n.start <= start);
    const last = nodes.findLast(n => n.start < end);
    const range = document.createRange();
    range.setStart(first.node, start - first.start);
    range.setEnd(last.node, end - last.start);
    ranges.push(range);
  }
  return ranges;
}

export function TranscriptFind({list, view, rows, expanded}) {
  const [open, setOpen] = useState(false), [query, setQuery] = useState('');
  const [ranges, setRanges] = useState([]), [index, setIndex] = useState(0);
  const input = useRef(null), previousFocus = useRef(null);

  useEffect(() => {
    const keydown = event => {
      if (event.key.toLowerCase() !== 'f' || !event.metaKey || event.altKey || event.ctrlKey) return;
      event.preventDefault();
      if (!open) previousFocus.current = document.activeElement;
      setOpen(true);
      input.current?.focus();
      input.current?.select?.();
    };
    document.addEventListener('keydown', keydown);
    return () => document.removeEventListener('keydown', keydown);
  }, [open]);

  useLayoutEffect(() => { if (open) input.current?.focus(); }, [open]);
  useLayoutEffect(() => {
    setRanges(open ? matchesIn(list.current, query) : []);
    setIndex(0);
  }, [open, query, rows, expanded]);
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
  const step = delta => { if (ranges.length) setIndex(i => (i + delta + ranges.length) % ranges.length); };
  if (!open) return null;
  return <div class="transcript-find" role="search" aria-label="Find in transcript" onKeyDown={event => {
    if (event.key === 'Escape') { event.preventDefault(); close(); }
    else if (event.key === 'Enter') { event.preventDefault(); step(event.shiftKey ? -1 : 1); }
  }}>
    <vscode-textfield ref={input} value={query} placeholder="Find" aria-label="Find in transcript" onInput={event => setQuery(event.currentTarget.value)} />
    <span class="transcript-find-count" aria-live="polite">{ranges.length ? `${index + 1} of ${ranges.length}` : 'No results'}</span>
    <button type="button" aria-label="Previous match" title="Previous match" onClick={() => step(-1)} disabled={!ranges.length}><vscode-icon name="arrow-up" /></button>
    <button type="button" aria-label="Next match" title="Next match" onClick={() => step(1)} disabled={!ranges.length}><vscode-icon name="arrow-down" /></button>
    <button type="button" aria-label="Close find" title="Close" onClick={close}><vscode-icon name="close" /></button>
  </div>;
}
