import {useState, type ReactNode} from 'react';
import {post} from '../api.ts';
import {Button} from '../components/ui/button';
import {Card, CardContent} from '../components/ui/card';
import {focusComposerField} from '../agent/focus-composer.ts';
import {useModels} from '../agent/catalogs.ts';
import {sessionOf} from '../sessions/selection.ts';
import {SelectionName} from '../agent/selection-name.tsx';
import {settingsText} from '../agent/selection-label.ts';
import {Icon} from '../shared/icon.tsx';
import {useStore} from '../store.ts';
import {Markdown} from './markdown.tsx';
import {Disclosure} from './disclosure.tsx';
import type {Session} from '../sessions/types.ts';
import type {ProposalItem as Proposal} from './types.ts';

interface ProposalItemProps {
  item: Proposal;
  session: Pick<Session, 'id' | 'ended'>;
  expanded: boolean;
  setOpen: (key: string, open: boolean) => void;
}

// Starting uses the saved proposal on the server. The browser sends only its ID.
// An open proposal is pinned below the history, so an answered one leaves its
// card and returns the focus to the composer.
// A fork works where this session works, so its card names no folder.
export function ProposalItem({item, session, expanded, setOpen}: ProposalItemProps) {
  const [state, store] = useStore();
  const [sending, setSending] = useState(false);
  const options = item.input;
  const models = useModels(sessionOf(state, session.id)?.workspace.id, options.settings.harness);
  const outcome = item.answer === 'Dismissed' ? 'Dismissed' : `Child session: ${options.name}`;
  const act = async (action: 'start-proposal' | 'dismiss-proposal') => {
    if (sending || session.ended) return;
    setSending(true);
    const result = await post(store, 'session', {action, id: session.id, proposal: item.id});
    if (result === null) setSending(false);
    else focusComposerField();
  };
  if (item.done) return (
    <Card className="border-0 py-3" aria-label="Proposed child session">
      <CardContent className="px-4">
        <Disclosure className="proposal-item" open={expanded} onToggle={open => setOpen(item.id, open)}
          summary={<><span className="truncate font-medium">{item.text}</span>
            <span className="flex min-w-0 items-center gap-4 text-xs text-muted-foreground"><Detail><span className="flex min-w-0 items-center gap-1.5" title={settingsText(options.settings, models)}><SelectionName settings={options.settings} models={models} /></span></Detail><span className="truncate">{outcome}</span></span></>}>
          {options.message && <Markdown text={options.message} style="chat" />}
        </Disclosure>
      </CardContent>
    </Card>
  );
  return (
    <div className="space-y-3" role="group" aria-label="Proposed child session">
      <div className="font-medium">{item.text}</div>
      <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted-foreground">
        <Detail><span className="flex min-w-0 items-center gap-1.5" title={settingsText(options.settings, models)}><SelectionName settings={options.settings} models={models} /></span></Detail>
        <Detail><Icon name="comment-discussion" />{options.fork ? 'Forked context' : 'Fresh context'}</Detail>
        {!options.fork && (options.dir
          ? <Detail><Icon name="folder-open" /><span className="truncate">{options.dir}</span></Detail>
          : <Detail><Icon name="git-branch" />New worktree</Detail>)}
      </div>
      {options.message && <div className="max-h-[40vh] overflow-y-auto"><Markdown text={options.message} style="chat" /></div>}
      <div className="flex gap-2">
        <Button size="sm" disabled={sending || session.ended} onClick={() => act('start-proposal')}>Start</Button>
        <Button size="sm" variant="secondary" disabled={sending || session.ended} onClick={() => act('dismiss-proposal')}>Dismiss</Button>
      </div>
    </div>
  );
}

// One icon-led fact about the proposed session.
function Detail({children}: {children: ReactNode}) {
  return <span className="flex min-w-0 items-center gap-1.5">{children}</span>;
}
