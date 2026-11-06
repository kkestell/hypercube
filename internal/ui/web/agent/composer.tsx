import {column} from '../app/workbench.ts';
import {shortcuts} from '../app/shortcuts.ts';
// The box below a transcript for the user's next message: its text, attached
// images, and attach and send buttons, and below them its profile and usage.
// A new session chooses its profile and worktree there; a running session can
// choose a profile of its harness. The text takes focus on mount.
import {useEffect, useRef, useState, type ChangeEvent, type ClipboardEvent, type KeyboardEvent} from 'react';
import {imageSrc, imageTypes} from '../images.ts';
import {post} from '../api.ts';
import {sendPrompt} from '../sessions/session-actions.ts';
import type {Session} from '../sessions/types.ts';
import {submitsOnKey} from '../shared/submit-key.ts';
import {useStore} from '../store.ts';
import {attachImages, canSend, draftOf, isUnsent, removeImage, typed} from './drafts.ts';
import {promptHistory, stepHistory, type History} from './history.ts';
import {MentionList, useMentions} from './use-mentions.tsx';
import {ProfileSelect} from './profile-select.tsx';
import {Usage} from './usage.tsx';
import {Limits} from './limits.tsx';
import {MergeButton} from './merge-button.tsx';
import {WorktreeSelect} from './worktree-select.tsx';
import {Button} from '../components/ui/button';
import {Card, CardContent} from '../components/ui/card';
import {Textarea} from '../components/ui/textarea';
import {Icon} from '../shared/icon.tsx';

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

  // Preserve keyboard focus in the file tab strip when a session mounts.
  useEffect(() => {
    if (!document.activeElement?.closest('.editor-tabs')) field.current?.focus();
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
  const stop = !session.ended && session.activity === 'busy' && !text.trim() && !draft.images.length && !draft.reading;
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
    <form className={`composer @container/composer my-4 shrink-0 ${column(state)}`} data-session={session.id} onSubmit={e => { e.preventDefault(); send(); }}>
      {isUnsent(session.id) && <WorktreeSelect />}
      {/* One line of text makes the box 50px tall: its border, padding, and the
          text's line; half that rounds its ends into half circles. */}
      <Card className="rounded-[25px] bg-transparent py-2 shadow-none dark:bg-input/30">
        <CardContent className="flex flex-col gap-1 px-3">
          {draft.images.length > 0 && (
            <div className="composer-images flex flex-wrap gap-2">
              {draft.images.map((image, i) => (
                <div key={i} className="composer-image relative">
                  <img className="size-16 object-cover" src={imageSrc(image) ?? undefined} alt="Attached image" />
                  <Button type="button" variant="secondary" size="icon" className="composer-image-remove absolute right-0 top-0" title="Remove Image" aria-label="Remove Image"
                    onClick={() => store.update(s => removeImage(s, session.id, image))}><Icon name="close" /></Button>
                </div>
              ))}
            </div>
          )}
          <MentionList mentions={mentions} />
          <div className="composer-input flex items-end gap-1">
            <Button type="button" variant="ghost" size="icon-sm" className="text-muted-foreground" title="Attach Image" aria-label="Attach Image" onClick={() => picker.current?.click()}><Icon name="attach" /></Button>
            <Textarea ref={field} rows={1} aria-label="Message" title={focusShortcut && `${focusShortcut.title} (${focusShortcut.keys[0]})`} className="composer-text flex-1 min-h-0 max-h-[calc(8lh+0.5rem)] resize-none rounded-none border-0 bg-transparent p-1 text-sm leading-6 shadow-none focus-visible:ring-0 dark:bg-transparent" value={text}
              role="combobox" aria-expanded={suggesting ? 'true' : undefined} aria-autocomplete="list"
              aria-controls={suggesting ? mentions.listId : undefined}
              aria-activedescendant={mentions.choices.length ? mentions.optionId : undefined}
              onChange={input} onKeyDown={key} onBlur={mentions.blur} onPaste={paste}
              onClick={e => mentions.sync(e.currentTarget)} onSelect={e => mentions.sync(e.currentTarget)}
              onKeyUp={e => mentions.sync(e.currentTarget)} />
            {stop
              ? <Button type="button" variant="ghost" size="icon-sm" title="Stop" aria-label="Stop" onClick={() => post(store, 'session', {action: 'interrupt', id: session.id})}><Icon name="stop" /></Button>
              // The paper plane's weight sits above its box's center; a pixel
              // down centers it by eye.
              : <Button type="submit" variant="ghost" size="icon-sm" title="Send" aria-label="Send" disabled={!canSend(state, session, text)}><Icon name="send" className="translate-y-px" /></Button>}
          </div>
          <input ref={picker} type="file" accept={imageTypes.join(',')} multiple hidden onChange={picked} />
        </CardContent>
      </Card>
      {/* The box's icons sit 21px in from its edges: its border, padding, and
          half the button around each icon. The profile button pads its icon
          10px and the 14px ring is centered on the send icon. */}
      <div className="composer-bar mt-2 flex flex-wrap items-center gap-x-2 gap-y-1 pl-[11px] pr-[22px]">
        <div className="composer-details flex min-w-32 flex-1 flex-wrap items-center gap-3 text-xs text-muted-foreground">
          <ProfileSelect session={session} />
          <MergeButton session={session} />
        </div>
        <div className="flex items-center gap-4">
          <Limits harness={session.harness} />
          <Usage usage={session.usage} />
        </div>
      </div>
    </form>
  );
}
