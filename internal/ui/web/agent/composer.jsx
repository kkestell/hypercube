// The box below a transcript for the user's next message: its text, attached
// images, the attach, stop, and send buttons, and a line with the session's
// harness, model, effort, and usage. The unsent session chooses its profile
// there instead. It takes the focus when it mounts.
import {useEffect, useRef} from 'preact/hooks';
import {post} from '../api.js';
import {imageSrc, imageTypes} from '../images.js';
import {sendPrompt} from '../sessions/session-actions.js';
import {AgentStatus} from '../shared/agent-status.jsx';
import {submitsOnKey} from '../shared/submit-key.js';
import {useStore} from '../store.js';
import {attachImages, canSend, draftOf, removeImage, typed, unsentKey} from './drafts.js';
import {ProfileSelect} from './profile-select.jsx';
import {Usage} from './usage.jsx';
import './composer.css';

export function Composer({session}) {
  const [state, store] = useStore();
  const draft = draftOf(state, session.id);
  const field = useRef(null), picker = useRef(null);

  // Preserve keyboard focus in the file tab strip when a session mounts.
  useEffect(() => {
    if (!document.activeElement?.closest('.editor-tabs')) field.current.focus();
  }, []);

  const send = () => sendPrompt(store, session);
  const key = e => {
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
      <textarea ref={field} rows={1} aria-label="Message" class="composer-text" value={draft.text}
        onInput={e => store.update(s => typed(s, session.id, e.currentTarget.value))} onKeyDown={key} />
      <input ref={picker} type="file" accept={imageTypes.join(',')} multiple hidden onChange={picked} />
      <div class="composer-bar">
        <vscode-button icon-only secondary icon="attach" title="Attach Image" aria-label="Attach Image" onClick={() => picker.current.click()} />
        <div class="composer-details">
          <AgentStatus session={session} />
          {session.id === unsentKey
            ? <ProfileSelect unsent={state.unsent} />
            : <span class="composer-config">{[session.harness, session.model, session.effort].filter(Boolean).join(' · ')}</span>}
          <Usage usage={session.usage} />
        </div>
        {session.activity === 'busy' && session.id !== unsentKey && (
          <vscode-button icon-only secondary icon="stop-circle" title="Stop" aria-label="Stop"
            onClick={() => post(store, 'session', {action: 'interrupt', id: session.id})} />
        )}
        <vscode-button icon-only secondary icon="send" title="Send" aria-label="Send" disabled={!canSend(state, session)} onClick={send} />
      </div>
    </form>
  );
}
