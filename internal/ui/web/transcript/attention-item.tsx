import {Card, CardContent} from '../components/ui/card';
import {Icon} from '../shared/icon.tsx';
// A session's question or approval request. While it is open it stands out
// and offers its answers; once answered it keeps just the question and answer
// in a labeled card.
import {AttentionForm} from './attention-form.tsx';
import {Markdown} from './markdown.tsx';
import type {RowControls} from './transcript-item.tsx';
import type {AttentionItem as Attention} from './types.ts';

export function AttentionItem({item, session}: Pick<RowControls, 'session'> & {item: Attention}) {
  if (!item.done) {
    return (
      <Card className="attention-item attention-item-open border-0"><CardContent className="space-y-3">
        <div className="attention-item-question"><Markdown text={item.text ?? ''} style="chat" /></div>
        <AttentionForm session={session.id} item={item} ended={session.ended} />
      </CardContent></Card>
    );
  }
  return (
    <Card className="attention-item gap-0 border-0 py-3"><CardContent className="space-y-2 px-4">
      <div className="flex items-center gap-2 text-xs font-medium text-muted-foreground"><Icon name="check" />{item.answer ? 'Answered' : 'Closed'}</div>
      <div className="attention-item-question attention-item-answered"><Markdown text={item.text ?? ''} style="chat" muted /></div>
      {item.answer && (
        <div className="attention-item-answer whitespace-pre-wrap break-words">{item.answer}</div>
      )}
    </CardContent></Card>
  );
}
