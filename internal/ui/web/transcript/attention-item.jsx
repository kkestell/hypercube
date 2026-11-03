// A session's question or approval request. While it is open it stands out
// and offers its answers; once answered it shows the question quietly, with
// the answer beside a check.
import {useStore} from '../store.js';
import {sessionOf} from '../sessions/selection.js';
import {AttentionForm} from './attention-form.jsx';
import './attention-item.css';

export function AttentionItem({item, session}) {
  const [state] = useStore();
  const sender = item.from && (sessionOf(state, item.from)?.session.title || item.from);
  const heading = sender && <div class="attention-item-source">{sender}</div>;
  if (!item.done) {
    return (
      <div class="attention-item attention-item-open">
        {heading}
        <div class="attention-item-question">{item.text ?? ''}</div>
        <AttentionForm session={item.from || session.id} item={item} />
      </div>
    );
  }
  return (
    <div class="attention-item">
      {heading}
      <div class="attention-item-question attention-item-answered">{item.text ?? ''}</div>
      {item.answer && (
        <div class="attention-item-answer">
          <vscode-icon name="check" />
          {item.answer}
        </div>
      )}
    </div>
  );
}
