// The box below a transcript for the user's next message: its text, attached
// images, the attach, stop, and send buttons, and a line with the agent's
// harness, model, effort, and usage. It takes the focus when it mounts.
import {useEffect, useRef} from 'preact/hooks';
import {post} from '../api.js';
import {imageSrc, imageTypes} from '../images.js';
import {sendPrompt} from '../sessions/session-actions.js';
import {AgentStatus} from '../shared/agent-status.jsx';
import {submitsOnKey} from '../shared/submit-key.js';
import {useStore} from '../store.js';
import {attachImages, canSend, draftOf, removeImage, typed, unsentKey} from './drafts.js';
import {Usage} from './usage.jsx';
import './composer.css';

export function Composer({agent}) {
  const [state, store] = useStore();
  const draft = draftOf(state, agent.id);
  const field = useRef(null), picker = useRef(null);

  // Clicking a tab, or moving between tabs with the arrow keys, mounts the
  // tab's composer; the focus stays on the tab.
  useEffect(() => {
    if (!document.activeElement?.closest('.editor-tabs')) field.current.focus();
  }, []);

  const send = () => sendPrompt(store, agent);
  const key = e => {
    if (!submitsOnKey(e)) return;
    e.preventDefault();
    send();
  };
  const picked = e => {
    const files = [...e.currentTarget.files];
    e.currentTarget.value = '';
    attachImages(store, agent.id, files);
  };

  return (
    <form class="composer" data-agent={agent.id} onSubmit={e => { e.preventDefault(); send(); }}>
      {draft.images.length > 0 && (
        <div class="composer-images">
          {draft.images.map(image => (
            <div class="composer-image">
              <img src={imageSrc(image)} alt="Attached image" />
              <vscode-button icon-only secondary icon="close" class="composer-image-remove" title="Remove Image" aria-label="Remove Image"
                onClick={() => store.update(s => removeImage(s, agent.id, image))} />
            </div>
          ))}
        </div>
      )}
      <textarea ref={field} rows={1} aria-label="Message" class="composer-text" value={draft.text}
        onInput={e => store.update(s => typed(s, agent.id, e.currentTarget.value))} onKeyDown={key} />
      <input ref={picker} type="file" accept={imageTypes.join(',')} multiple hidden onChange={picked} />
      <div class="composer-bar">
        <vscode-button icon-only secondary icon="attach" title="Attach Image" aria-label="Attach Image" onClick={() => picker.current.click()} />
        <div class="composer-details">
          {agent.role === 'lead' && <AgentStatus agent={agent} />}
          <span class="composer-config">{[agent.harness, agent.model, agent.effort].filter(Boolean).join(' · ')}</span>
          <Usage usage={agent.usage} />
        </div>
        {agent.activity === 'busy' && agent.id !== unsentKey && (
          <vscode-button icon-only secondary icon="stop-circle" title="Stop" aria-label="Stop"
            onClick={() => post(store, 'agent', {action: 'interrupt', id: agent.id})} />
        )}
        <vscode-button icon-only secondary icon="send" title="Send" aria-label="Send" disabled={!canSend(state, agent)} onClick={send} />
      </div>
    </form>
  );
}
