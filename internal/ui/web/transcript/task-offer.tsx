// Task placement is an immediate action, not a question to fill out and submit.
import {useState} from 'react';
import {answerRequest} from '../api.ts';
import {cleared} from '../agent/drafts.ts';
import {Button} from '../components/ui/button';
import {Icon} from '../shared/icon.tsx';
import {useStore} from '../store.ts';
import {Markdown} from './markdown.tsx';
import type {AttentionItem} from './types.ts';

export function TaskOffer({session, item, ended}: {session: string; item: AttentionItem; ended?: boolean}) {
  const [, store] = useStore();
  const [sending, setSending] = useState(false);
  const locked = sending || !!ended;
  const answer = async (choice: string) => {
    if (locked) return;
    setSending(true);
    if (await answerRequest(store, session, item.id, [choice], '')) {
      store.update(state => cleared(state, `${session}/${item.id}`));
    } else setSending(false);
  };

  return <div className="space-y-3">
    <div className="flex items-start justify-between gap-3">
      <div className="attention-item-question min-w-0"><Markdown text={item.text ?? ''} style="chat" /></div>
      <Button type="button" variant="ghost" size="icon-xs" className="shrink-0" aria-label="Dismiss task" title="Dismiss task" disabled={locked} onClick={() => answer('Dismiss')}><Icon name="close" /></Button>
    </div>
    <div className="flex gap-2">
      <Button type="button" size="sm" disabled={locked} onClick={() => answer('New session')}>New session</Button>
      <Button type="button" variant="secondary" size="sm" disabled={locked} onClick={() => answer('Do it here')}>Do it here</Button>
    </div>
  </div>;
}
