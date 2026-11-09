// The home view: every signed-in account's pull requests that are its move,
// across all repositories, with a warning for each account that could not be
// read.
import {Icon} from '../shared/icon.tsx';
import {useStore} from '../store.ts';
import {PullRequestRow} from './pull-request-row.tsx';
import {yourMoves} from './turns.ts';

const sentence = (text: string) => text.endsWith('.') ? text : text + '.';

const time = (ms: number) => new Date(ms).toLocaleTimeString([], {hour: 'numeric', minute: '2-digit'});

export function AllPullRequests() {
  const [{accounts}] = useStore();
  const moves = yourMoves(accounts);
  const failed = accounts.filter(account => account.error);
  const checking = !failed.length && accounts.every(account => !account.updated);
  return <section aria-label="Your move across accounts" className="min-h-0 flex-1 overflow-auto p-6">
    <h1 className="mb-4 text-lg font-medium">Pull Requests</h1>
    {failed.map(({login, error, updated}) => <p key={login} className="mb-2 flex items-center gap-1.5 text-sm text-muted-foreground">
      <Icon name="warning" className="text-destructive" />
      <span>Could not read pull requests for {login}: {sentence(error!.split('\n')[0])}{updated > 0 && ` Showing data from ${time(updated)}.`}</span>
    </p>)}
    {checking ? <p className="text-sm text-muted-foreground">Checking GitHub.</p>
      : moves.length ? <ul aria-label="Your move" className="space-y-2">
        {moves.map(({account, pr, turn}) => <PullRequestRow key={`${account}\n${pr.url}`} pr={pr} turn={turn} account={account} place={<>
          In <a href={`https://github.com/${pr.repo}`} target="_blank" rel="noopener noreferrer" className="underline underline-offset-2">{pr.repo}</a> for {account}.
        </>} />)}
      </ul>
      : <p className="text-sm text-muted-foreground">Nothing needs you.</p>}
  </section>;
}
