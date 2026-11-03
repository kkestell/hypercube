// The answers to an open attention request: content-sized choice buttons
// with their notes, and a labeled reply field with a separate Send button
// when the request takes typed text. The transcript and notification cards
// both draw it, and answers go only through answerRequest and alwaysAllow. An
// approval request also offers to always allow its tool. Its controls are
// disabled while an answer is sent and enabled again if the server refuses it.
// A typed answer is a draft, so it survives the form closing.
import {useId, useState} from 'preact/hooks';
import {alwaysAllow, answerRequest} from '../api.js';
import {draftOf, typed} from '../agent/drafts.js';
import {submitsOnKey} from '../shared/submit-key.js';
import {useStore} from '../store.js';
import './attention-form.css';

export function AttentionForm({session, item}) {
  const [state, store] = useStore();
  const [sending, setSending] = useState(false);
  const id = useId();
  const key = `${session}/${item.id}`;
  const text = draftOf(state, key).text;

  const submit = async answer => {
    if (sending) return;
    setSending(true);
    if (await answer()) store.update(s => typed(s, key, ''));
    else setSending(false);
  };
  const send = choice => submit(() => answerRequest(store, session, item.id, choice, text));
  const allowAlways = all => submit(() => alwaysAllow(store, session, item.id, all));
  const sendText = () => {
    if (text.trim()) send('');
  };

  return (
    <div class="attention-form">
      {item.choices?.length > 0 && (
        <div class="attention-form-choices" role="group" aria-label="Choices">
          {item.choices.map((choice, i) => (
            <div class="attention-form-choice" key={choice}>
              <vscode-button secondary data-choice={choice} disabled={sending} aria-describedby={item.notes?.[i] ? `${id}-note-${i}` : undefined} onClick={() => send(choice)}>
                {choice}
              </vscode-button>
              {item.notes?.[i] && <span class="attention-form-note" id={`${id}-note-${i}`}>{item.notes[i]}</span>}
            </div>
          ))}
        </div>
      )}
      {item.grant && (
        <div class="attention-form-choices" role="group" aria-label="Always allow">
          <vscode-button secondary data-always="session" disabled={sending} onClick={() => allowAlways(false)}>Always allow in this session</vscode-button>
          <vscode-button secondary data-always="all" disabled={sending} onClick={() => allowAlways(true)}>Always allow in all sessions</vscode-button>
        </div>
      )}
      {item.free && (
        <div class="attention-form-reply">
          <vscode-label for={`${id}-reply`}>{item.choices?.length ? 'Other response' : 'Your response'}</vscode-label>
          <div class="attention-form-entry">
            <vscode-textfield
              id={`${id}-reply`}
              label={item.choices?.length ? 'Other response' : 'Your response'}
              value={text}
              disabled={sending}
              onInput={e => store.update(s => typed(s, key, e.currentTarget.value))}
              onKeyDown={e => e.target === e.currentTarget && submitsOnKey(e) && sendText()}
            />
            <vscode-button secondary disabled={sending || !text.trim()} onClick={sendText}>Send</vscode-button>
          </div>
        </div>
      )}
    </div>
  );
}
