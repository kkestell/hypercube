import {Card, CardContent} from '../components/ui/card';
import {Icon} from '../shared/icon.tsx';
// A session's question or approval request. While it is open it offers its
// answers inside the pending card; once answered it becomes a collapsed row.
import {AttentionForm} from './attention-form.tsx';
import {Disclosure} from './disclosure.tsx';
import {Markdown} from './markdown.tsx';
import type {RowControls} from './transcript-item.tsx';
import type {AttentionItem as Attention} from './types.ts';

export function AttentionItem({item, session}: Pick<RowControls, 'session'> & {item: Attention}) {
  return (
    <div className="attention-item attention-item-open space-y-3">
      <div className="attention-item-question max-h-[40vh] overflow-y-auto"><Markdown text={item.text ?? ''} style="chat" /></div>
      <AttentionForm session={session.id} item={item} ended={session.ended} />
    </div>
  );
}

// AnsweredItem is a finished question, laid out as a finished tool call: its
// status, its outcome, and the answer or question beside it. Opening it shows
// the question and the answer.
export function AnsweredItem({item, expanded, setOpen}: {item: Attention; expanded: boolean; setOpen: (key: string, open: boolean) => void}) {
  const answer = item.answer ?? '';
  const title = <span className="min-w-0 truncate text-sm" title={answer || item.text}>
    <span>{answer ? 'Answered' : 'Closed'}</span> <span className="ml-1 text-xs text-muted-foreground">{answer || item.text}</span>
  </span>;
  return (
    <Disclosure className="attention-item [:is(.tool-items-call,.attention-item)[data-state=closed]+&]:-mt-3" open={expanded} onToggle={open => setOpen(item.id, open)}
      marker={<Icon name="check" role="img" title={answer ? 'Answered' : 'Closed'} />} summary={title}>
      {expanded && (
        <Card className="mt-2 gap-0 overflow-hidden border-0 py-0">
          <CardContent className="space-y-2 px-3 py-2">
            <h4 className="text-xs font-medium text-muted-foreground">Question</h4>
            <div className="attention-item-question"><Markdown text={item.text ?? ''} style="chat" muted /></div>
          </CardContent>
          {answer && <CardContent className="space-y-2 border-t px-3 py-2">
            <h4 className="text-xs font-medium text-muted-foreground">Answer</h4>
            <div className="attention-item-answer whitespace-pre-wrap break-words">{answer}</div>
          </CardContent>}
        </Card>
      )}
    </Disclosure>
  );
}
