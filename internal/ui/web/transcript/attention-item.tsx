import {Card, CardContent} from '../components/ui/card';
import {Icon} from '../shared/icon.tsx';
// A session's question or approval request. While it is open it stands out
// and offers its answers; once answered it shows the question quietly, with
// the answer beside a check. Completed copies from children stay hidden.
import {useStore} from '../store.ts';
import {sessionOf} from '../sessions/selection.ts';
import {AttentionForm} from './attention-form.tsx';
import {Markdown} from './markdown.tsx';
import type {RowControls} from './transcript-item.tsx';
import type {AttentionItem as Attention} from './types.ts';

export function AttentionItem({item, session}: Pick<RowControls, 'session'> & {item: Attention}) {
  const [state] = useStore();
  if (item.from && item.done) return null;
  const sender = item.from && (sessionOf(state, item.from)?.session.title || item.from);
  const heading = sender && <div className="attention-item-source text-sm font-medium">{sender}</div>;
  if (!item.done) {
    return (
      <Card className="attention-item attention-item-open"><CardContent className="space-y-3">
        {heading}
        <div className="attention-item-question"><Markdown text={item.text ?? ''} style="chat" /></div>
        <AttentionForm session={item.from || session.id} item={item} />
      </CardContent></Card>
    );
  }
  return (
    <div className="attention-item space-y-3">
      {heading}
      <div className="attention-item-question attention-item-answered text-muted-foreground"><Markdown text={item.text ?? ''} style="chat" /></div>
      {item.answer && (
        <div className="attention-item-answer flex items-center gap-2">
          <Icon name="check" />
          {item.answer}
        </div>
      )}
    </div>
  );
}
