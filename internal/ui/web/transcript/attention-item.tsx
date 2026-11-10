// A session's question or approval request. While an approval or task offer
// is open it offers its answers inside the pending card, where QuestionsForm
// answers questions; once answered it becomes a collapsed row.
import type {ReactNode} from 'react';
import type {Session} from '../sessions/types.ts';
import {Icon} from '../shared/icon.tsx';
import {AttentionForm} from './attention-form.tsx';
import {Disclosure} from './disclosure.tsx';
import {Markdown} from './markdown.tsx';
import {TaskOffer} from './task-offer.tsx';
import {TranscriptDetails} from './transcript-details.tsx';
import type {AttentionItem as Attention} from './types.ts';

export function AttentionItem({item, session}: {item: Attention; session: Pick<Session, 'id' | 'ended'>}) {
  return (
    <div className="space-y-3">
      {item.offer ? <TaskOffer session={session.id} item={item} ended={session.ended} /> : <>
        <Markdown text={item.text ?? ''} style="chat" />
        <AttentionForm session={session.id} item={item} ended={session.ended} />
      </>}
    </div>
  );
}

// AnsweredItem is a finished question, laid out as a finished tool call: its
// status, its outcome, and the answer or question beside it. Opening it shows
// the question and the answer.
export function AnsweredItem({item}: {item: Attention}) {
  const answer = item.answer ?? '';
  const outcome = answer ? 'Answered' : 'Closed';
  const title = <span className="min-w-0 truncate text-sm" title={answer || item.text}>
    {outcome} <span className="ml-1 text-xs text-muted-foreground">{answer || item.text}</span>
  </span>;
  const sections: [string, ReactNode][] = [['Question', <Markdown text={item.text ?? ''} style="chat" muted />]];
  if (answer) sections.push(['Answer', <div className="whitespace-pre-wrap break-words">{answer}</div>]);
  return (
    <Disclosure id={item.id} className="attention-item [:is(.tool-items-call,.attention-item)[data-state=closed]+&]:-mt-3"
      marker={<Icon name="check" role="img" title={outcome} />} summary={title}>
      <div className="mt-2"><TranscriptDetails sections={sections} /></div>
    </Disclosure>
  );
}
