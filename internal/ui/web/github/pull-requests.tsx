// A GitHub workspace's open pull requests that need the account, then the ones
// it waits on, with a Review button that sends a requested review to a
// session.
import {useState} from 'react';
import type {Workspace} from '../sessions/types.ts';
import {PullRequestRow} from './pull-request-row.tsx';
import {SendDialog} from './send-dialog.tsx';
import {sortTurns} from './turns.ts';
import type {GitHubStatus, PullRequest} from './types.ts';

function emptyText({account, updated, error}: GitHubStatus): string {
  if (!account) return 'Choose an account to see pull requests.';
  if (!updated) return error ? 'Could not load pull requests.' : 'Checking GitHub.';
  return '';
}

export function PullRequests({workspace}: {workspace: Workspace}) {
  const [sending, setSending] = useState<PullRequest | null>(null);
  const status = workspace.github;
  if (!status) return null;
  const empty = emptyText(status);
  const {mine, waiting} = sortTurns(status.pullRequests, status.account);
  const rows = (label: string, turns: typeof mine) => <ul aria-label={label} className="space-y-2">
    {turns.map(({pr, turn}) => <PullRequestRow key={pr.url} pr={pr} turn={turn} account={status.account} onReview={() => setSending(pr)} />)}
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
      <p className="mt-4 px-3 text-sm">
        <a href={`${status.url}/pulls`} target="_blank" rel="noopener noreferrer" className="underline underline-offset-2">All open pull requests</a>
      </p>
    </>}
    {sending && <SendDialog workspace={workspace} pr={sending} onClose={() => setSending(null)} />}
  </section>;
}
