// The box below a transcript for the user's next message: its text, attached
// images, and attach and send buttons, and below them its settings and usage.
// A new session chooses its settings and worktree there; a running session can
// change its model and effort. The text takes focus on mount.
import {useEffect, useRef, useState, type ChangeEvent, type ClipboardEvent, type KeyboardEvent} from 'react';
import {firstKeys, shortcuts} from '../app/shortcuts.ts';
import {imageSrc, imageTypes} from '../images.ts';
import {post} from '../api.ts';
import {sendPrompt} from '../sessions/session-actions.ts';
import type {Session} from '../sessions/types.ts';
import {submitsOnKey} from '../shared/submit-key.ts';
import {useStore} from '../store.ts';
import {registerComposer} from './focus-composer.ts';
import {attachImages, canSend, draftOf, isUnsent, removeImage, typed} from './drafts.ts';
import {promptHistory, stepHistory, type History} from './history.ts';
import {MentionList, useMentions} from './use-mentions.tsx';
import {SessionSelection} from './session-selection.tsx';
import {Usage} from './usage.tsx';
import {Limits} from './limits.tsx';
import {WorktreeSelect} from './worktree-select.tsx';
import {Card, CardContent} from '../components/ui/card';
import {Textarea} from '../components/ui/textarea';
import {IconButton} from '../shared/icon-button.tsx';

export function Composer({session}: {session: Session}) {
  const [state, store] = useStore();
  const draft = draftOf(state, session.id);
  const field = useRef<HTMLTextAreaElement>(null), picker = useRef<HTMLInputElement>(null);
  const [history, setHistory] = useState<History | null>(null);
  const text = history === null ? draft.text : history.edits[history.index];
  const changeText = (value: string) => {
    if (history === null) store.update(s => typed(s, session.id, value));
    else setHistory({...history, edits: {...history.edits, [history.index]: value}});
  };
  const mentions = useMentions(field, text, changeText, session.id, state.unsent?.workspace);
  const input = (e: ChangeEvent<HTMLTextAreaElement>) => { changeText(e.currentTarget.value); mentions.sync(e.currentTarget); };

  // The text takes the focus when a session mounts, unless the focus is on a
  // tab strip.
  useEffect(() => {
    const element = field.current;
    if (!element) return;
    if (!document.activeElement?.closest('[role=tablist]')) element.focus();
    return registerComposer(element);
  }, []);

  const send = () => {
    if (!canSend(state, session, text)) return;
    if (history !== null) store.update(s => typed(s, session.id, text));
    setHistory(null);
    mentions.close();
    sendPrompt(store, session);
  };
  const key = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.nativeEvent.isComposing) return;
    const plain = !e.altKey && !e.ctrlKey && !e.metaKey && !e.shiftKey;
    if (mentions.keydown(e)) return;
    if (plain && e.currentTarget.selectionStart === e.currentTarget.selectionEnd) {
      const pos = e.currentTarget.selectionStart;
      const up = e.key === 'ArrowUp' && !text.slice(0, pos).includes('\n');
      const down = e.key === 'ArrowDown' && !text.slice(pos).includes('\n');
      if (up || (down && history !== null)) {
        const transcript = state.session?.id === session.id ? state.session.transcript : null;
        const prompts = promptHistory(transcript);
        if (prompts.length) {
          e.preventDefault();
          const next = stepHistory(prompts, history, up ? 'up' : 'down');
          mentions.dismiss(next === null ? draft.text : next.edits[next.index]);
          setHistory(next);
          return;
        }
      }
    }
    if (!submitsOnKey(e)) return;
    e.preventDefault();
    send();
  };
  const stop = !isUnsent(session.id) && !session.ended && session.activity === 'busy' && !text.trim() && !draft.images.length && !draft.reading;
  const paste = (e: ClipboardEvent<HTMLTextAreaElement>) => {
    const files = [...e.clipboardData.files].filter(file => file.type.startsWith('image/'));
    if (files.length) attachImages(store, session.id, files);
  };
  const picked = (e: ChangeEvent<HTMLInputElement>) => {
    const files = [...e.currentTarget.files ?? []];
    e.currentTarget.value = '';
    attachImages(store, session.id, files);
  };

  const focusShortcut = shortcuts.find(s => s.id === 'composer');
  const suggesting = mentions.choices.length > 0 || !!mentions.error;

  return (
    <form className="composer @container/composer relative my-4 shrink-0 px-4" data-session={session.id} onSubmit={e => { e.preventDefault(); send(); }}>
      {isUnsent(session.id) && <WorktreeSelect />}
      {/* The text field draws no focus ring of its own; the box shows focus. */}
      <Card className="rounded-2xl py-2 focus-within:border-ring focus-within:ring-[3px] focus-within:ring-ring/50">
        <CardContent className="flex flex-col gap-1 px-3">
          {draft.images.length > 0 && (
            <div className="flex flex-wrap gap-2">
              {draft.images.map((image, i) => (
                <div key={i} className="relative">
                  <img className="size-16 object-cover" src={imageSrc(image) ?? undefined} alt="Attached image" />
                  <IconButton icon="close" label="Remove Image" type="button" variant="secondary" className="absolute right-0 top-0"
                    onClick={() => store.update(s => removeImage(s, session.id, image))} />
                </div>
              ))}
            </div>
          )}
          <MentionList mentions={mentions} />
          <div className="flex items-end gap-1">
            <IconButton icon="attach" label="Attach Image" type="button" className="mb-1 text-muted-foreground" onClick={() => picker.current?.click()} />
            <Textarea ref={field} rows={1} aria-label="Message" title={focusShortcut && `${focusShortcut.title} (${firstKeys('composer')})`} className="composer-text flex-1 min-h-0 max-h-[calc(8lh+0.5rem)] resize-none rounded-none border-0 bg-transparent p-1 text-sm leading-6 shadow-none focus-visible:ring-0 dark:bg-transparent" value={text}
              role="combobox" aria-expanded={suggesting ? 'true' : undefined} aria-autocomplete="list"
              aria-controls={suggesting ? mentions.listId : undefined}
              aria-activedescendant={mentions.choices.length ? mentions.optionId : undefined}
              onChange={input} onKeyDown={key} onBlur={mentions.blur} onPaste={paste}
              onClick={e => mentions.sync(e.currentTarget)} onSelect={e => mentions.sync(e.currentTarget)}
              onKeyUp={e => mentions.sync(e.currentTarget)} />
            {stop
              ? <IconButton icon="stop" label="Stop" type="button" className="mb-1" onClick={() => post(store, 'session', {action: 'interrupt', id: session.id})} />
              : <IconButton icon="send" label="Send" type="submit" className="mb-1" disabled={!canSend(state, session, text)} />}
          </div>
          <input ref={picker} type="file" accept={imageTypes.join(',')} multiple hidden onChange={picked} />
        </CardContent>
      </Card>
      <div className="mt-2 flex flex-wrap items-center gap-x-2 gap-y-1 px-1">
        <div className="flex min-w-32 flex-1 flex-wrap items-center gap-3 text-xs text-muted-foreground">
          <SessionSelection session={session} />
        </div>
        <div className="flex items-center gap-4">
          <Limits harness={session.harness} />
          <Usage usage={session.usage} />
        </div>
      </div>
    </form>
  );
}
