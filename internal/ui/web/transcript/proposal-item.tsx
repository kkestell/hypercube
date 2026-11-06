import {useRef, useState} from 'react';
import {post} from '../api.ts';
import {Button} from '../components/ui/button';
import {Card, CardContent} from '../components/ui/card';
import {useStore} from '../store.ts';
import {Markdown} from './markdown.tsx';
import type {ProposalItem as Proposal} from './types.ts';

// Starting uses the saved proposal on the server. The browser sends only its ID.
// A fork works where this session works, so its card names no folder.
export function ProposalItem({item, session}: {item: Proposal; session: string}) {
  const [, store] = useStore();
  const [sending, setSending] = useState(false);
  const card = useRef<HTMLDivElement>(null);
  const options = item.input;
  const act = async (action: 'start-proposal' | 'dismiss-proposal') => {
    if (sending) return;
    setSending(true);
    const result = await post(store, 'session', {action, id: session, proposal: item.id});
    if (result === null) setSending(false);
    else card.current?.focus();
  };
  return (
    <Card ref={card} tabIndex={-1} className="border-0 py-3" aria-label="Proposed child session">
      <CardContent className="space-y-3 px-4">
        <div className="font-medium">{item.text}</div>
        <div className="text-xs text-muted-foreground">
          {options.profile.name} · {options.fork ? 'Forked context' : `Fresh context · ${options.dir ? `Folder: ${options.dir}` : 'New worktree'}`}
        </div>
        {options.message && <Markdown text={options.message} style="chat" />}
        {item.done ? (
          <div className="text-xs text-muted-foreground">{item.answer === 'Dismissed' ? 'Dismissed' : `Child session: ${options.name}`}</div>
        ) : (
          <div className="flex gap-2">
            <Button size="sm" disabled={sending} onClick={() => act('start-proposal')}>Start</Button>
            <Button size="sm" variant="secondary" disabled={sending} onClick={() => act('dismiss-proposal')}>Dismiss</Button>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
