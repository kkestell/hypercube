import {useEffect, useLayoutEffect, useMemo, useRef, useState, type KeyboardEvent, type RefObject} from 'react';
import {Command, CommandItem, CommandList} from '../components/ui/command';
import {mentionFiles} from '../api.ts';
import {unsentKey} from './drafts.ts';
import {insertMention, mentionAt, mentionChoices, prepareFiles, type MentionFile} from './mention.ts';

// Selection is the text and caret together, so suggestions always correspond
// to the position from the same browser event.
interface Selection {
  text: string;
  start: number;
  end: number;
}

// Mentions is the suggestion list for the @ token at the caret and the
// handlers that drive it.
export interface Mentions {
  choices: string[];
  active: number;
  optionRef: (option: HTMLElement | null) => void;
  listRef: (list: HTMLElement | null) => void;
  optionId: string | undefined;
  listId: string | undefined;
  error: string;
  sync: (target: HTMLTextAreaElement) => void;
  close: () => void;
  dismiss: (value: string) => void;
  blur: () => void;
  choose: (path: string) => void;
  keydown: (e: KeyboardEvent<HTMLTextAreaElement>) => boolean;
}

export function useMentions(
  field: RefObject<HTMLTextAreaElement | null>,
  text: string,
  replace: (value: string) => void,
  session: string,
  workspace: string | null | undefined,
): Mentions {
  const [selection, setSelection] = useState<Selection | null>(null);
  const [dismissed, setDismissed] = useState<string | null>(null);
  const [selected, setSelected] = useState(0);
  const [files, setFiles] = useState<MentionFile[]>([]);
  const [error, setError] = useState('');
  const caret = useRef<number | null>(null);
  const [activeOption, setActiveOption] = useState<HTMLElement | null>(null), [list, setList] = useState<HTMLElement | null>(null);
  const candidate = selection && mentionAt(selection.text, selection.start, selection.end);
  const token = candidate && `${candidate.start}:${selection.text}`;
  const mention = selection?.text === text && token !== dismissed ? candidate : null;
  const key = mention ? `${mention.start}:${mention.query}` : '';
  const choices = useMemo(() => mention ? mentionChoices(files, mention.query) : [], [files, mention?.query]);
  const active = Math.min(selected, Math.max(0, choices.length - 1));
  const open = !!mention;

  useLayoutEffect(() => {
    const params = session === unsentKey ? (workspace ? {workspace} : null) : {session};
    if (!params) { setFiles([]); setError(''); return; }
    if (!open) return;
    const controller = new AbortController();
    setError('');
    mentionFiles(params, controller.signal)
      .then(paths => { if (!controller.signal.aborted) setFiles(prepareFiles(paths)); })
      .catch((err: unknown) => { if (!controller.signal.aborted) setError('Could not list files: ' + (err instanceof Error ? err.message : String(err))); });
    return () => controller.abort();
  }, [open, session, workspace]);
  useLayoutEffect(() => {
    if (caret.current === null) return;
    field.current?.setSelectionRange(caret.current, caret.current);
    caret.current = null;
  });
  useLayoutEffect(() => { setSelected(0); }, [key]);
  useEffect(() => { activeOption?.scrollIntoView?.({block: 'nearest'}); }, [active, choices[active], activeOption]);

  const sync = (target: HTMLTextAreaElement) => {
    const next = {text: target.value, start: target.selectionStart, end: target.selectionEnd};
    const found = mentionAt(next.text, next.start, next.end);
    const current = found && `${found.start}:${next.text}`;
    setDismissed(previous => current === previous ? previous : null);
    setSelection(next);
  };
  const close = () => { setDismissed(token); setSelection(null); };
  const dismiss = (value: string) => {
    const found = mentionAt(value, value.length, value.length);
    setDismissed(found ? `${found.start}:${value}` : null);
    setSelection(null);
  };
  const blur = () => { setSelection(null); setDismissed(null); };
  const choose = (path: string) => {
    if (!mention) return;
    const value = insertMention(text, mention, path);
    const position = mention.start + path.length + 1;
    setDismissed(`${mention.start}:${value}`);
    setSelection({text: value, start: position, end: position});
    caret.current = position;
    replace(value);
    field.current?.focus();
  };
  const keydown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
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
  return {
    choices, active, optionRef: setActiveOption, listRef: setList,
    optionId: activeOption?.id, listId: list?.id, error: open ? error : '',
    sync, close, dismiss, blur, choose, keydown,
  };
}

export function MentionList({mentions}: {mentions: Mentions}) {
  const {choices, active, optionRef, listRef, error, choose} = mentions;
  if (!choices.length && !error) return null;
  return <Command shouldFilter={false} value={choices[active] ?? ''} className="composer-mentions">
    <CommandList ref={listRef} aria-label="Files">
      {error && <CommandItem disabled>{error}</CommandItem>}
      {choices.map((path, i) => <CommandItem key={path} value={path} ref={i === active ? optionRef : undefined}
        onMouseDown={e => e.preventDefault()} onSelect={() => choose(path)}>{path}</CommandItem>)}
    </CommandList>
  </Command>;
}
