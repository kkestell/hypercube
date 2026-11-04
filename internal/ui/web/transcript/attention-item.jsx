import {Card, CardContent} from '../components/ui/card';
import {Icon} from '../shared/icon.jsx';
// A session's question or approval request. While it is open it stands out
// and offers its answers; once answered it shows the question quietly, with
// the answer beside a check. Completed copies from children stay hidden.
import {useStore} from '../store.js';
import {sessionOf} from '../sessions/selection.js';
import {AttentionForm} from './attention-form.jsx';

export function AttentionItem({item, session}) {
  const [state] = useStore();
  if (item.from && item.done) return null;
  const sender = item.from && (sessionOf(state, item.from)?.session.title || item.from);
  const heading = sender && <div className="attention-item-source text-sm font-medium">{sender}</div>;
  if (!item.done) {
    return (
      <Card className="attention-item attention-item-open"><CardContent className="space-y-3">
        {heading}
        <div className="attention-item-question whitespace-pre-wrap break-words">{item.text ?? ''}</div>
        <AttentionForm session={item.from || session.id} item={item} />
      </CardContent></Card>
    );
  }
  return (
    <div className="attention-item space-y-3">
      {heading}
      <div className="attention-item-question attention-item-answered whitespace-pre-wrap text-muted-foreground">{item.text ?? ''}</div>
      {item.answer && (
        <div className="attention-item-answer flex items-center gap-2">
          <Icon name="check" />
          {item.answer}
        </div>
      )}
    </div>
  );
}
