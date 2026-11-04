// The box below a transcript for the user's next message: its text, attached
// images, attach and send buttons, and its profile and usage. A new session
// chooses its profile and worktree there; after the first prompt the profile
// shows as text, and a worktree session shows its branch beside it. The text
// takes focus on mount.
import {useEffect, useRef, useState} from 'react';
import {imageSrc, imageTypes} from '../images.js';
import {post} from '../api.js';
import {sendPrompt} from '../sessions/session-actions.js';
import {AgentStatus} from '../shared/agent-status.jsx';
import {submitsOnKey} from '../shared/submit-key.js';
import {useStore} from '../store.js';
import {attachImages, canSend, draftOf, removeImage, typed, unsentKey} from './drafts.js';
import {promptHistory} from './history.js';
import {MentionList, useMentions} from './use-mentions.jsx';
import {ProfileSelect} from './profile-select.jsx';
import {Usage} from './usage.jsx';
import {SessionBranch} from './session-branch.jsx';
import {WorktreeToggle} from './worktree-toggle.jsx';
import {Button} from '../components/ui/button';
import {Card, CardContent} from '../components/ui/card';
import {Textarea} from '../components/ui/textarea';
import {Icon} from '../shared/icon.jsx';

export function Composer({session}) {
  const [state, store] = useStore();
  const draft = draftOf(state, session.id);
  const field = useRef(null), picker = useRef(null);
  const [history, setHistory] = useState(null);
  const text = history === null ? draft.text : history.edits[history.index];
  const changeText = value => {
    if (history === null) store.update(s => typed(s, session.id, value));
    else setHistory({...history, edits: {...history.edits, [history.index]: value}});
  };
  const mentions = useMentions(field, text, changeText, session.id, state.unsent?.workspace);
  const input = e => { changeText(e.currentTarget.value); mentions.sync(e.currentTarget); };

  // Preserve keyboard focus in the file tab strip when a session mounts.
  useEffect(() => {
    if (!document.activeElement?.closest('.editor-tabs')) field.current.focus();
  }, []);

  const send = () => {
    if (!canSend(state, session, text)) return;
    if (history !== null) store.update(s => typed(s, session.id, text));
    setHistory(null);
    mentions.close();
    sendPrompt(store, session);
  };
  const key = e => {
    if (e.isComposing || e.nativeEvent?.isComposing) return;
    const plain = !e.altKey && !e.ctrlKey && !e.metaKey && !e.shiftKey;
    if (mentions.keydown(e)) return;
    if (plain && e.currentTarget.selectionStart === e.currentTarget.selectionEnd) {
      const pos = e.currentTarget.selectionStart;
      const up = e.key === 'ArrowUp' && !text.slice(0, pos).includes('\n');
      const down = e.key === 'ArrowDown' && history !== null && !text.slice(pos).includes('\n');
      if (up || down) {
        const transcript = state.session?.id === session.id ? state.session.transcript : null;
        const prompts = promptHistory(transcript);
        if (prompts.length) {
          e.preventDefault();
          const index = up ? (history === null ? prompts.length - 1 : Math.max(0, history.index - 1)) : history.index + 1;
          const edits = {...Object.fromEntries(prompts.map((value, i) => [i, value])), ...history?.edits};
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
  const stop = session.running && !session.ended && session.activity === 'busy' && !text && !draft.images.length && !draft.reading;
  const paste = e => {
    const files = [...e.clipboardData.files].filter(file => file.type.startsWith('image/'));
    if (files.length) attachImages(store, session.id, files);
  };
  const picked = e => {
    const files = [...e.currentTarget.files];
    e.currentTarget.value = '';
    attachImages(store, session.id, files);
  };

  return (
    <form className="composer m-4 shrink-0" data-session={session.id} onSubmit={e => { e.preventDefault(); send(); }}>
      <Card className="py-3">
        <CardContent className="flex flex-col gap-3 px-3">
          {draft.images.length > 0 && (
            <div className="composer-images flex flex-wrap gap-2">
              {draft.images.map((image, i) => (
                <div key={i} className="composer-image relative">
                  <img className="size-16 object-cover" src={imageSrc(image)} alt="Attached image" />
                  <Button type="button" variant="secondary" size="icon" className="composer-image-remove absolute right-0 top-0" title="Remove Image" aria-label="Remove Image"
                    onClick={() => store.update(s => removeImage(s, session.id, image))}><Icon name="close" /></Button>
                </div>
              ))}
            </div>
          )}
          <MentionList mentions={mentions} />
          <Textarea ref={field} rows={1} aria-label="Message" title="Focus Composer (⌘L)" className="composer-text max-h-40 resize-none" value={text}
            role="combobox" aria-expanded={mentions.choices.length || mentions.error ? 'true' : undefined} aria-autocomplete="list"
            aria-controls={mentions.choices.length || mentions.error ? mentions.listId : undefined}
            aria-activedescendant={mentions.choices.length ? mentions.optionId : undefined}
            onChange={input} onKeyDown={key} onBlur={mentions.blur} onPaste={paste}
            onClick={e => mentions.sync(e.currentTarget)} onSelect={e => mentions.sync(e.currentTarget)}
            onKeyUp={e => mentions.sync(e.currentTarget)} />
          <input ref={picker} type="file" accept={imageTypes.join(',')} multiple hidden onChange={picked} />
          <div className="composer-bar flex items-center gap-2">
            <Button type="button" variant="ghost" size="icon" title="Attach Image" aria-label="Attach Image" onClick={() => picker.current.click()}><Icon name="attach" /></Button>
            <div className="composer-details flex min-w-0 flex-1 flex-wrap items-center gap-3 text-sm">
              <AgentStatus session={session} />
              <ProfileSelect session={session} />
              {session.id === unsentKey ? <WorktreeToggle /> : <SessionBranch session={session} />}
              <Usage usage={session.usage} />
            </div>
            {stop
              ? <Button type="button" size="icon" title="Stop" aria-label="Stop" onClick={() => post(store, 'session', {action: 'interrupt', id: session.id})}><Icon name="stop" /></Button>
              : <Button type="submit" size="icon" title="Send" aria-label="Send" disabled={!canSend(state, session, text)}><Icon name="send" /></Button>}
          </div>
        </CardContent>
      </Card>
    </form>
  );
}
