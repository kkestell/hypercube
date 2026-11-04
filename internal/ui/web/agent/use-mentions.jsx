import {useEffect, useLayoutEffect, useMemo, useRef, useState} from 'preact/hooks';
import {mentionFiles} from '../api.js';
import {unsentKey} from './drafts.js';
import {insertMention, mentionAt, mentionChoices, prepareFiles} from './mention.js';

// The selection records the text and caret together, so suggestions always
// correspond to the position from the same browser event.
export function useMentions(field, text, replace, session, workspace) {
  const [selection, setSelection] = useState(null);
  const [dismissed, setDismissed] = useState(null);
  const [selected, setSelected] = useState(0);
  const [files, setFiles] = useState([]);
  const [error, setError] = useState('');
  const caret = useRef(null), activeOption = useRef(null);
  const candidate = selection && mentionAt(selection.text, selection.start, selection.end);
  const token = candidate && `${candidate.start}:${selection.text}`;
  const mention = selection?.text === text && token !== dismissed ? candidate : null;
  const key = mention ? `${mention.start}:${mention.query}` : '';
  const choices = useMemo(() => mention ? mentionChoices(files, mention.query) : [], [files, mention?.query]);
  const active = Math.min(selected, Math.max(0, choices.length - 1));
  const open = !!mention;

  useLayoutEffect(() => {
    if (session === unsentKey && !workspace) { setFiles([]); setError(''); return; }
    if (!open) return;
    const controller = new AbortController();
    setError('');
    mentionFiles(session === unsentKey ? {workspace} : {session}, controller.signal)
      .then(paths => { if (!controller.signal.aborted) setFiles(prepareFiles(paths)); })
      .catch(err => { if (!controller.signal.aborted) setError('Could not list files: ' + err.message); });
    return () => controller.abort();
  }, [open, session, workspace]);
  useLayoutEffect(() => {
    if (caret.current !== null) {
      field.current.setSelectionRange(caret.current, caret.current);
      caret.current = null;
    }
  });
  useLayoutEffect(() => { setSelected(0); }, [key]);
  useEffect(() => { activeOption.current?.scrollIntoView?.({block: 'nearest'}); }, [active, choices[active]]);

  const sync = target => {
    const next = {text: target.value, start: target.selectionStart, end: target.selectionEnd};
    const found = mentionAt(next.text, next.start, next.end);
    const current = found && `${found.start}:${next.text}`;
    setDismissed(previous => current === previous ? previous : null);
    setSelection(next);
  };
  const close = () => { setDismissed(token); setSelection(null); };
  const dismiss = value => {
    const found = mentionAt(value, value.length, value.length);
    setDismissed(found ? `${found.start}:${value}` : null);
    setSelection(null);
  };
  const blur = () => { setSelection(null); setDismissed(null); };
  const choose = path => {
    if (!mention) return;
    const value = insertMention(text, mention, path);
    const position = mention.start + path.length + 1;
    setDismissed(`${mention.start}:${value}`);
    setSelection({text: value, start: position, end: position});
    caret.current = position;
    replace(value);
    field.current.focus();
  };
  const keydown = e => {
    if (!mention || e.altKey || e.ctrlKey || e.metaKey || e.shiftKey) return false;
    if (e.key === 'Escape') { e.preventDefault(); close(); return true; }
    if (!choices.length) return false;
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault();
      setSelected((active + (e.key === 'ArrowDown' ? 1 : -1) + choices.length) % choices.length);
      return true;
    }
    if (e.key === 'Enter' || e.key === 'Tab') { e.preventDefault(); choose(choices[active]); return true; }
    return false;
  };
  return {choices, active, activeOption, error: open ? error : '', sync, close, dismiss, blur, choose, keydown};
}

export function MentionList({mentions}) {
  const {choices, active, activeOption, error, choose} = mentions;
  if (!choices.length && !error) return null;
  return <div class="composer-mentions" role="listbox" id="composer-mentions" aria-label="Files">
    {error && <div role="option" aria-disabled="true">{error}</div>}
    {choices.map((path, i) => <div role="option" ref={i === active ? activeOption : undefined} id={'composer-file-' + i} aria-selected={i === active}
      onMouseDown={e => { e.preventDefault(); choose(path); }}>{path}</div>)}
  </div>;
}
