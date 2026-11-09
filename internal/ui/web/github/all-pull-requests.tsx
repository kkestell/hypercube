// The home view: every signed-in account's pull requests that are its move,
// across all repositories, with a warning for each account that could not be
// read.
import {useState} from 'react';
import {Button} from '../components/ui/button';
import {Icon} from '../shared/icon.tsx';
import {useStore} from '../store.ts';
import {CloneDialog} from './clone-dialog.tsx';
import {tracks} from './clone.ts';
import {PullRequestRow} from './pull-request-row.tsx';
import {yourMoves} from './turns.ts';
import {failureText} from './wording.ts';
import type {PullRequest} from './types.ts';

export function AllPullRequests() {
  const [{accounts, workspaces}] = useStore();
  const [cloning, setCloning] = useState<{pr: PullRequest; account: string} | null>(null);
  const moves = yourMoves(accounts);
  const failed = accounts.filter(account => account.error);
  const checking = !failed.length && accounts.every(account => !account.updated);
  return <section aria-label="Your move across accounts" className="min-h-0 flex-1 overflow-auto p-6">
    <h1 className="mb-4 text-lg font-medium">Pull Requests</h1>
    {failed.map(({login, error, updated}) => <p key={login} className="mb-2 flex items-center gap-1.5 text-sm text-muted-foreground">
      <Icon name="warning" className="text-destructive" />
      <span>{failureText(`Could not read pull requests for ${login}`, error!, updated)}</span>
    </p>)}
    {cloning && <CloneDialog pr={cloning.pr} account={cloning.account} onClose={() => setCloning(null)} />}
    {checking ? <p className="text-sm text-muted-foreground">Checking GitHub.</p>
      : moves.length ? <ul aria-label="Your move" className="space-y-2">
        {moves.map(({account, pr, turn}) => <PullRequestRow key={`${account}\n${pr.url}`} pr={pr} turn={turn} account={account} place={<>
          In <a href={`https://github.com/${pr.repo}`} target="_blank" rel="noopener noreferrer" className="underline underline-offset-2">{pr.repo}</a> for {account}.
        </>} trailing={tracks(workspaces, pr.repo) ? null
          : <Button variant="secondary" size="sm" aria-label={`Clone ${pr.repo}`} onClick={() => setCloning({pr, account})}>Clone</Button>} />)}
      </ul>
      : <p className="text-sm text-muted-foreground">Nothing needs you.</p>}
  </section>;
}
