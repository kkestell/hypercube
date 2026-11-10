// A proposed child session. Starting uses the saved proposal on the server;
// the browser sends only its ID. An open proposal is pinned below the history,
// so an answered one leaves its card and returns the focus to the composer.
// A fork works where this session works, so its card names no folder.
import type {ReactNode} from 'react';
import {post} from '../api.ts';
import {Button} from '../components/ui/button';
import {Card, CardContent} from '../components/ui/card';
import {focusComposerField} from '../agent/focus-composer.ts';
import {SelectionName} from '../agent/selection-name.tsx';
import {settingsText} from '../agent/selection-label.ts';
import type {Session} from '../sessions/types.ts';
import {Icon} from '../shared/icon.tsx';
import {useStore} from '../store.ts';
import {Disclosure} from './disclosure.tsx';
import {Markdown} from './markdown.tsx';
import type {ProposalItem as Proposal} from './types.ts';
import {useSubmit} from './use-submit.ts';

export function ProposalItem({item, session}: {item: Proposal; session: Pick<Session, 'id' | 'ended'>}) {
  const [, store] = useStore();
  const [sending, submit] = useSubmit();
  const options = item.input;
  const act = (action: 'start-proposal' | 'dismiss-proposal') => submit(async () => {
    if (await post(store, 'session', {action, id: session.id, proposal: item.id}) === null) return false;
    focusComposerField();
    return true;
  });
  return (
    <div className="space-y-3" role="group" aria-label="Proposed child session">
      <div className="font-medium">{item.text}</div>
      <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted-foreground">
        <Settings item={item} />
        <Detail><Icon name="comment-discussion" />{options.fork ? 'Forked context' : 'Fresh context'}</Detail>
        {!options.fork && (options.dir
          ? <Detail><Icon name="folder-open" /><span className="truncate">{options.dir}</span></Detail>
          : <Detail><Icon name="git-branch" />New worktree</Detail>)}
      </div>
      {options.message && <Markdown text={options.message} style="chat" />}
      <div className="flex gap-2">
        <Button size="sm" disabled={sending || session.ended} onClick={() => act('start-proposal')}>Start</Button>
        <Button size="sm" variant="secondary" disabled={sending || session.ended} onClick={() => act('dismiss-proposal')}>Dismiss</Button>
      </div>
    </div>
  );
}

// AnsweredProposal is a started or dismissed proposal in the history, which
// opens to show its brief.
export function AnsweredProposal({item}: {item: Proposal}) {
  const options = item.input;
  const outcome = item.answer === 'Dismissed' ? 'Dismissed' : `Child session: ${options.name}`;
  return (
    <Card className="py-3" aria-label="Proposed child session">
      <CardContent className="px-4">
        <Disclosure id={item.id}
          summary={<><span className="truncate font-medium">{item.text}</span>
            <span className="flex min-w-0 items-center gap-4 text-xs text-muted-foreground"><Settings item={item} /><span className="truncate">{outcome}</span></span></>}>
          {options.message && <Markdown text={options.message} style="chat" />}
        </Disclosure>
      </CardContent>
    </Card>
  );
}

// One icon-led fact about the proposed session.
function Detail({children}: {children: ReactNode}) {
  return <span className="flex min-w-0 items-center gap-1.5">{children}</span>;
}

function Settings({item}: {item: Proposal}) {
  const [state] = useStore();
  return <Detail><span className="flex min-w-0 items-center gap-1.5" title={settingsText(item.input.settings, state.config?.models ?? null)}><SelectionName settings={item.input.settings} /></span></Detail>;
}
