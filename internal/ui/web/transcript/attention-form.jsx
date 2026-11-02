// The answers to an open attention request: a button per choice and, when the
// request takes typed text, a field and Send. The transcript and notification
// cards both draw it, and answers go only through answerRequest. Its controls
// are disabled while an answer is sent and enabled again if the server
// refuses it. A typed answer is a draft, so it survives the form closing.
import {useState} from 'preact/hooks';
import {answerRequest} from '../api.js';
import {draftOf, typed} from '../agent/drafts.js';
import {submitsOnKey} from '../agent/composer.jsx';
import {useStore} from '../store.js';
import './attention-form.css';

export function AttentionForm({agent, item}) {
  const [state, store] = useStore();
  const [sending, setSending] = useState(false);
  const key = `${agent}/${item.id}`;
  const text = draftOf(state, key).text;

  const send = async choice => {
    if (sending) return;
    setSending(true);
    if (await answerRequest(agent, item.id, choice, text)) store.update(s => typed(s, key, ''));
    else setSending(false);
  };
  const sendText = () => {
    if (text.trim()) send('');
  };

  return (
    <div class="attention-form">
      {item.free && (
        <vscode-textfield
          class="attention-form-text"
          value={text}
          disabled={sending}
          onInput={e => store.update(s => typed(s, key, e.currentTarget.value))}
          onKeyDown={e => submitsOnKey(e) && sendText()}
        />
      )}
      {(item.choices ?? []).map(choice => (
        <vscode-button key={choice} disabled={sending} onClick={() => send(choice)}>{choice}</vscode-button>
      ))}
      {item.free && <vscode-button disabled={sending} onClick={sendText}>Send</vscode-button>}
    </div>
  );
}
