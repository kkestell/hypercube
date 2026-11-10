import {useEffect, useLayoutEffect, useMemo, useRef, useState, type KeyboardEvent, type RefObject} from 'react';
import {mentionFiles} from '../api.ts';
import {Command, CommandItem, CommandList} from '../components/ui/command';
import {useRequest} from '../shared/use-request.ts';
import {isUnsent} from './drafts.ts';
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

const noFiles: MentionFile[] = [];

export function useMentions(
  field: RefObject<HTMLTextAreaElement | null>,
  text: string,
  replace: (value: string) => void,
  session: string,
  workspace: string | null | undefined,
): Mentions {
  const [selection, setSelection] = useState<Selection | null>(null);
  const [dismissed, setDismissed] = useState<string | null>(null);
  // The highlighted choice, kept for the mention and query it was picked in.
  const [selected, setSelected] = useState({key: '', index: 0});
  const caret = useRef<number | null>(null);
  const [activeOption, setActiveOption] = useState<HTMLElement | null>(null), [list, setList] = useState<HTMLElement | null>(null);
  const candidate = selection && mentionAt(selection.text, selection.start, selection.end);
  const token = candidate && `${candidate.start}:${selection.text}`;
  const mention = selection?.text === text && token !== dismissed ? candidate : null;
  const key = mention ? `${mention.start}:${mention.query}` : '';
  const open = !!mention;
  const params = isUnsent(session) ? (workspace ? {workspace} : null) : {session};
  const request = useRequest(open && params ? async signal => prepareFiles(await mentionFiles(params, signal)) : null, [open, session, workspace]);
  // Each opening loads the files again, and the last list stays while it
  // loads, so a choice typed quickly is still there to pick.
  const [files, setFiles] = useState(noFiles);
  const loaded = params ? request.data ?? files : noFiles;
  if (loaded !== files) setFiles(loaded);
  const choices = useMemo(() => mention ? mentionChoices(files, mention.query) : [], [files, mention?.query]);
  const active = selected.key === key ? Math.min(selected.index, Math.max(0, choices.length - 1)) : 0;

  useLayoutEffect(() => {
    if (caret.current === null) return;
    field.current?.setSelectionRange(caret.current, caret.current);
    caret.current = null;
  });
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
      setSelected({key, index: (active + (e.key === 'ArrowDown' ? 1 : -1) + choices.length) % choices.length});
      return true;
    }
    if (e.key === 'Enter' || e.key === 'Tab') { e.preventDefault(); choose(choices[active]); return true; }
    return false;
  };
  return {
    choices, active, optionRef: setActiveOption, listRef: setList,
    optionId: activeOption?.id, listId: list?.id, error: request.error && 'Could not list files: ' + request.error,
    sync, close, dismiss, blur, choose, keydown,
  };
}

export function MentionList({mentions}: {mentions: Mentions}) {
  const {choices, active, optionRef, listRef, error, choose} = mentions;
  if (!choices.length && !error) return null;
  return <Command shouldFilter={false} value={choices[active] ?? ''}>
    <CommandList ref={listRef} aria-label="Files">
      {error && <CommandItem disabled>{error}</CommandItem>}
      {choices.map((path, i) => <CommandItem key={path} value={path} ref={i === active ? optionRef : undefined}
        onMouseDown={e => e.preventDefault()} onSelect={() => choose(path)}>{path}</CommandItem>)}
    </CommandList>
  </Command>;
}
