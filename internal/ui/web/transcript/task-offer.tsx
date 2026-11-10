// Task placement is an immediate action, not a question to fill out and submit.
import {answerRequest} from '../api.ts';
import {cleared} from '../agent/drafts.ts';
import {Button} from '../components/ui/button';
import {IconButton} from '../shared/icon-button.tsx';
import {useStore} from '../store.ts';
import {Markdown} from './markdown.tsx';
import type {AttentionItem} from './types.ts';
import {useSubmit} from './use-submit.ts';

export function TaskOffer({session, item, ended}: {session: string; item: AttentionItem; ended?: boolean}) {
  const [, store] = useStore();
  const [sending, submit] = useSubmit();
  const locked = sending || !!ended;
  const answer = (choice: string) => submit(async () => {
    const sent = await answerRequest(store, session, item.id, [choice], '');
    if (sent) store.update(state => cleared(state, `${session}/${item.id}`));
    return sent;
  });

  return <div className="space-y-3">
    <div className="flex items-start justify-between gap-3">
      <div className="min-w-0"><Markdown text={item.text ?? ''} style="chat" /></div>
      <IconButton icon="close" label="Dismiss task" type="button" className="shrink-0" disabled={locked} onClick={() => answer('Dismiss')} />
    </div>
    <div className="flex gap-2">
      <Button type="button" size="sm" disabled={locked} onClick={() => answer('New session')}>New session</Button>
      <Button type="button" variant="secondary" size="sm" disabled={locked} onClick={() => answer('Do it here')}>Do it here</Button>
    </div>
  </div>;
}
