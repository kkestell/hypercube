// A GitHub workspace's open pull requests, with the actions that send work on
// them to a session.
import {useState} from 'react';
import {Button} from '../components/ui/button';
import type {Workspace} from '../sessions/types.ts';
import {actionLabels, actionsFor, type PullRequestAction} from './instructions.ts';
import {PullRequestIcon} from './pull-request-icon.tsx';
import {SendDialog} from './send-dialog.tsx';
import type {GitHubStatus, PullRequest} from './types.ts';
import {pullRequestFacts} from './wording.ts';

function emptyText({account, updated, error, pullRequests}: GitHubStatus): string {
  if (!account) return 'Choose an account to see pull requests.';
  if (!updated) return error ? 'Could not load pull requests.' : 'Checking GitHub.';
  return pullRequests.length ? '' : 'No open pull requests.';
}

export function PullRequests({workspace}: {workspace: Workspace}) {
  const [sending, setSending] = useState<{pr: PullRequest; action: PullRequestAction} | null>(null);
  const status = workspace.github;
  if (!status) return null;
  const empty = emptyText(status);
  const rows = empty ? [] : status.pullRequests;
  return <section className="mb-6">
    <h2 className="mb-2 text-sm font-medium">Pull Requests</h2>
    {empty ? <p className="text-sm text-muted-foreground">{empty}</p> : <ul aria-label="Pull requests" className="space-y-2">
      {rows.map(pr => <li key={pr.number} className="flex items-center gap-2 px-3 text-sm">
        <PullRequestIcon pr={pr} />
        <div className="min-w-0 flex-1">
          <a href={pr.url} target="_blank" rel="noopener noreferrer" className="block truncate underline underline-offset-2">#{pr.number} {pr.title}</a>
          <p className="flex flex-wrap gap-x-1.5 text-muted-foreground">{[pr.author, ...pullRequestFacts(pr)].map(part => <span key={part}>{part}</span>)}</p>
        </div>
        {actionsFor(pr).map(action => <Button key={action} variant="secondary" size="sm" onClick={() => setSending({pr, action})}>{actionLabels[action]}</Button>)}
      </li>)}
    </ul>}
    {!empty && status.open > rows.length && <p className="mt-2 px-3 text-sm">
      <a href={`${status.url}/pulls`} target="_blank" rel="noopener noreferrer" className="underline underline-offset-2">View all {status.open} open pull requests on GitHub</a>
    </p>}
    {sending && <SendDialog workspace={workspace} {...sending} onClose={() => setSending(null)} />}
  </section>;
}
