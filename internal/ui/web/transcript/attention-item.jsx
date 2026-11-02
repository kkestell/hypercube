// An agent's question or approval request. While it is open it stands out
// and offers its answers; once answered it shows the question quietly, with
// the answer beside a check.
import {memo} from 'preact/compat';
import {AttentionForm} from './attention-form.jsx';
import './attention-item.css';

export const AttentionItem = memo(({item, agent}) => {
  if (!item.done) {
    return (
      <div class="attention-item attention-item-open">
        <div class="attention-item-question">{item.text ?? ''}</div>
        <AttentionForm agent={agent} item={item} />
      </div>
    );
  }
  return (
    <div class="attention-item">
      <div class="attention-item-question attention-item-answered">{item.text ?? ''}</div>
      {item.answer && (
        <div class="attention-item-answer">
          <vscode-icon name="check" />
          {item.answer}
        </div>
      )}
    </div>
  );
});
