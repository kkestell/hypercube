// A GitHub workspace's open pull requests that need the account, then the ones
// it waits on, with the actions that send work on its own pull requests to a
// session.
import {useState} from 'react';
import {Button} from '../components/ui/button';
import type {Workspace} from '../sessions/types.ts';
import {actionLabels, actionsFor, type PullRequestAction} from './instructions.ts';
import {PullRequestIcon} from './pull-request-icon.tsx';
import {SendDialog} from './send-dialog.tsx';
import {sortTurns, type Turn} from './turns.ts';
import type {GitHubStatus, PullRequest} from './types.ts';

function emptyText({account, updated, error, open}: GitHubStatus): string {
  if (!account) return 'Choose an account to see pull requests.';
  if (!updated) return error ? 'Could not load pull requests.' : 'Checking GitHub.';
  return open ? '' : 'No open pull requests.';
}

type Send = (pr: PullRequest, action: PullRequestAction) => void;

function Row({pr, turn, account, send}: {pr: PullRequest; turn: Turn; account: string; send: Send}) {
  const own = pr.author === account;
  return <li className={`flex items-center gap-2 px-3 text-sm ${turn.yours ? '' : 'opacity-60'}`}>
    <PullRequestIcon pr={pr} />
    <div className="min-w-0 flex-1">
      <a href={pr.url} target="_blank" rel="noopener noreferrer" className="block truncate underline underline-offset-2">#{pr.number} {pr.title}</a>
      <p className="flex flex-wrap gap-x-1.5 text-muted-foreground">
        <span>{turn.reason}</span>
        {!own && pr.author && <span>From {pr.author}.</span>}
      </p>
    </div>
    {own && actionsFor(pr).map(action => <Button key={action} variant="secondary" size="sm" onClick={() => send(pr, action)}>{actionLabels[action]}</Button>)}
  </li>;
}

export function PullRequests({workspace}: {workspace: Workspace}) {
  const [sending, setSending] = useState<{pr: PullRequest; action: PullRequestAction} | null>(null);
  const status = workspace.github;
  if (!status) return null;
  const empty = emptyText(status);
  const {mine, waiting} = sortTurns(status.pullRequests, status.account);
  const others = status.open - mine.length - waiting.length;
  const send: Send = (pr, action) => setSending({pr, action});
  const rows = (label: string, turns: typeof mine) => <ul aria-label={label} className="space-y-2">
    {turns.map(({pr, turn}) => <Row key={pr.number} pr={pr} turn={turn} account={status.account} send={send} />)}
  </ul>;
  return <section className="mb-6">
    <h2 className="mb-2 text-sm font-medium">Pull Requests</h2>
    {empty ? <p className="text-sm text-muted-foreground">{empty}</p> : <>
      <h3 className="mb-2 text-xs font-medium text-muted-foreground">Your move</h3>
      {mine.length ? rows('Your move', mine) : <p className="px-3 text-sm text-muted-foreground">Nothing needs you.</p>}
      {waiting.length > 0 && <>
        <h3 className="mt-4 mb-2 text-xs font-medium text-muted-foreground">Waiting on others</h3>
        {rows('Waiting on others', waiting)}
      </>}
      {others > 0 && <p className="mt-4 px-3 text-sm">
        <a href={`${status.url}/pulls`} target="_blank" rel="noopener noreferrer" className="underline underline-offset-2">{others} other open {others === 1 ? 'pull request' : 'pull requests'}</a>
      </p>}
    </>}
    {sending && <SendDialog workspace={workspace} {...sending} onClose={() => setSending(null)} />}
  </section>;
}
