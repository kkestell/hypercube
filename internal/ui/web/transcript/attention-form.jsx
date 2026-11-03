// The answers to an open attention request, as VS Code's Quick Pick: a list of
// choices, each with its note beside it, and, when the request takes typed
// text, a field with Send inside it. The transcript and notification cards
// both draw it, and answers go only through answerRequest. Its controls are
// disabled while an answer is sent and enabled again if the server refuses it.
// A typed answer is a draft, so it survives the form closing.
import {useState} from 'preact/hooks';
import {answerRequest} from '../api.js';
import {draftOf, typed} from '../agent/drafts.js';
import {submitsOnKey} from '../shared/submit-key.js';
import {useStore} from '../store.js';
import './attention-form.css';

export function AttentionForm({session, item}) {
  const [state, store] = useStore();
  const [sending, setSending] = useState(false);
  const key = `${session}/${item.id}`;
  const text = draftOf(state, key).text;

  const send = async choice => {
    if (sending) return;
    setSending(true);
    if (await answerRequest(store, session, item.id, choice, text)) store.update(s => typed(s, key, ''));
    else setSending(false);
  };
  const sendText = () => {
    if (text.trim()) send('');
  };

  // The tree reports a click or Enter on a choice as a selection.
  const chosen = e => {
    const choice = e.detail[0]?.dataset.choice;
    if (choice !== undefined) send(choice);
  };

  return (
    <div class="attention-form">
      {item.choices?.length > 0 && (
        <vscode-tree class="attention-form-choices" aria-label="Choices" aria-disabled={sending} onvsc-tree-select={chosen}>
          {item.choices.map((choice, i) => (
            <vscode-tree-item key={choice} data-choice={choice}>
              {choice}
              {item.notes?.[i] && <span slot="description">{item.notes[i]}</span>}
            </vscode-tree-item>
          ))}
        </vscode-tree>
      )}
      {item.free && (
        <vscode-textfield
          label={item.text ? 'Answer to ' + item.text : 'Answer'}
          value={text}
          disabled={sending}
          onInput={e => store.update(s => typed(s, key, e.currentTarget.value))}
          onKeyDown={e => e.target === e.currentTarget && submitsOnKey(e) && sendText()}
        >
          <vscode-toolbar-button slot="content-after" icon="send" label="Send" title="Send" disabled={sending || !text.trim()} onClick={sendText} />
        </vscode-textfield>
      )}
    </div>
  );
}
