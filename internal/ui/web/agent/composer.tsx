import {column} from '../app/workbench.ts';
import {shortcuts} from '../app/shortcuts.ts';
// The box below a transcript for the user's next message: its text, attached
// images, attach and send buttons, and its profile and usage. A new session
// chooses its profile and worktree there; a running session can choose a
// profile of its harness, and a worktree shows its branch beside it. The text
// takes focus on mount.
import {useEffect, useRef, useState, type ChangeEvent, type ClipboardEvent, type KeyboardEvent} from 'react';
import {imageSrc, imageTypes} from '../images.ts';
import {post} from '../api.ts';
import {sendPrompt} from '../sessions/session-actions.ts';
import type {Session} from '../sessions/types.ts';
import {submitsOnKey} from '../shared/submit-key.ts';
import {useStore} from '../store.ts';
import {attachImages, canSend, draftOf, removeImage, typed, unsentKey} from './drafts.ts';
import {promptHistory} from './history.ts';
import {MentionList, useMentions} from './use-mentions.tsx';
import {ProfileSelect} from './profile-select.tsx';
import {Usage} from './usage.tsx';
import {SessionBranch} from './session-branch.tsx';
import {WorktreeToggle} from './worktree-toggle.tsx';
import {Button} from '../components/ui/button';
import {Card, CardContent} from '../components/ui/card';
import {Textarea} from '../components/ui/textarea';
import {Icon} from '../shared/icon.tsx';

// History is the prompt being edited while the arrow keys step through the
// session's earlier prompts, with the edits made to each so far.
interface History {
  index: number;
  edits: Record<number, string>;
}

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
          const index = history === null ? prompts.length - 1 : up ? Math.max(0, history.index - 1) : history.index + 1;
          const edits: Record<number, string> = {...prompts, ...history?.edits};
          mentions.dismiss(index >= prompts.length ? draft.text : edits[index]);
          if (index >= prompts.length) setHistory(null);
          else setHistory({index, edits});
          return;
        }
      }
    }
    if (!submitsOnKey(e)) return;
    e.preventDefault();
    send();
  };
  const stop = session.running && !session.ended && session.activity === 'busy' && !text.trim() && !draft.images.length && !draft.reading;
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
    <form className={`composer m-4 shrink-0 ${column(state)}`} data-session={session.id} onSubmit={e => { e.preventDefault(); send(); }}>
      <Card className="py-3">
        <CardContent className="flex flex-col gap-3 px-3">
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
          <Textarea ref={field} rows={1} aria-label="Message" title={focusShortcut && `${focusShortcut.title} (${focusShortcut.keys[0]})`} className="composer-text max-h-40 resize-none" value={text}
            role="combobox" aria-expanded={suggesting ? 'true' : undefined} aria-autocomplete="list"
            aria-controls={suggesting ? mentions.listId : undefined}
            aria-activedescendant={mentions.choices.length ? mentions.optionId : undefined}
            onChange={input} onKeyDown={key} onBlur={mentions.blur} onPaste={paste}
            onClick={e => mentions.sync(e.currentTarget)} onSelect={e => mentions.sync(e.currentTarget)}
            onKeyUp={e => mentions.sync(e.currentTarget)} />
          <input ref={picker} type="file" accept={imageTypes.join(',')} multiple hidden onChange={picked} />
          <div className="composer-bar flex items-center gap-2">
            <Button type="button" variant="ghost" size="icon-sm" title="Attach Image" aria-label="Attach Image" onClick={() => picker.current?.click()}><Icon name="attach" /></Button>
            <div className="composer-details flex min-w-0 flex-1 flex-wrap items-center gap-3 text-sm">
              <ProfileSelect session={session} />
              {session.id === unsentKey ? <WorktreeToggle /> : <SessionBranch session={session} />}
            </div>
            <Usage usage={session.usage} />
            {stop
              ? <Button type="button" size="icon-sm" title="Stop" aria-label="Stop" onClick={() => post(store, 'session', {action: 'interrupt', id: session.id})}><Icon name="stop" /></Button>
              : <Button type="submit" size="icon-sm" title="Send" aria-label="Send" disabled={!canSend(state, session, text)}><Icon name="send" /></Button>}
          </div>
        </CardContent>
      </Card>
    </form>
  );
}
