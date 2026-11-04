// The box below a transcript for the user's next message: its text, attached
// images, attach and send buttons, and its profile and usage. A new session
// chooses its profile and worktree there; after the first prompt the profile
// shows as text, and a worktree session shows its branch beside it. The text
// takes focus on mount.
import {useEffect, useRef, useState} from 'preact/hooks';
import {imageSrc, imageTypes} from '../images.js';
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
import './composer.css';

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
    if (e.isComposing) return;
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
  const picked = e => {
    const files = [...e.currentTarget.files];
    e.currentTarget.value = '';
    attachImages(store, session.id, files);
  };

  return (
    <form class="composer" data-session={session.id} onSubmit={e => { e.preventDefault(); send(); }}>
      {draft.images.length > 0 && (
        <div class="composer-images">
          {draft.images.map(image => (
            <div class="composer-image">
              <img src={imageSrc(image)} alt="Attached image" />
              <vscode-button icon-only secondary icon="close" class="composer-image-remove" title="Remove Image" aria-label="Remove Image"
                onClick={() => store.update(s => removeImage(s, session.id, image))} />
            </div>
          ))}
        </div>
      )}
      <MentionList mentions={mentions} />
      <textarea ref={field} rows={1} aria-label="Message" title="Focus Composer (⌘L)" class="composer-text" value={text}
        role="combobox" aria-expanded={mentions.choices.length || mentions.error ? 'true' : undefined} aria-autocomplete="list"
        aria-controls={mentions.choices.length || mentions.error ? 'composer-mentions' : undefined}
        aria-activedescendant={mentions.choices.length ? 'composer-file-' + mentions.active : undefined}
        onInput={input} onKeyDown={key} onBlur={mentions.blur}
        onClick={e => mentions.sync(e.currentTarget)} onSelect={e => mentions.sync(e.currentTarget)}
        onKeyUp={e => mentions.sync(e.currentTarget)} />
      <input ref={picker} type="file" accept={imageTypes.join(',')} multiple hidden onChange={picked} />
      <div class="composer-bar">
        <vscode-button icon-only secondary icon="attach" title="Attach Image" aria-label="Attach Image" onClick={() => picker.current.click()} />
        <div class="composer-details">
          <AgentStatus session={session} />
          <ProfileSelect session={session} />
          {session.id === unsentKey ? <WorktreeToggle /> : <SessionBranch session={session} />}
          <Usage usage={session.usage} />
        </div>
        <vscode-button icon-only secondary icon="send" title="Send" aria-label="Send" disabled={!canSend(state, session, text)} onClick={send} />
      </div>
    </form>
  );
}
