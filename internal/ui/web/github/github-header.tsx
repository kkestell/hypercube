// What a GitHub workspace's dashboard shows under its name: the repository,
// the account in use, the branch's tracking, and why the data may be old.
import {post} from '../api.ts';
import {Select, SelectContent, SelectItem, SelectTrigger, SelectValue} from '../components/ui/select';
import type {Workspace} from '../sessions/types.ts';
import {Icon} from '../shared/icon.tsx';
import {useStore, type Store} from '../store.ts';
import {trackingText} from './wording.ts';

// chooseAccount saves the account a workspace's new agents and terminals use.
export const chooseAccount = (store: Store, workspace: string, account: string) =>
  post(store, 'workspace', {action: 'github-account', id: workspace, account});

const time = (ms: number) => new Date(ms).toLocaleTimeString([], {hour: 'numeric', minute: '2-digit'});

export function GitHubHeader({workspace}: {workspace: Workspace}) {
  const [, store] = useStore();
  const status = workspace.github;
  if (!status) return null;
  return <div className="mb-6 space-y-1 text-sm text-muted-foreground">
    <p><a href={status.url} target="_blank" rel="noopener noreferrer" className="underline underline-offset-2">{status.repo}</a></p>
    {status.choices
      ? <div className="space-y-1">
        <Select value={status.account} onValueChange={account => chooseAccount(store, workspace.id, account)}>
          <SelectTrigger aria-label="GitHub account" size="sm"><SelectValue placeholder="Choose an account" /></SelectTrigger>
          <SelectContent>{status.choices.map(account => <SelectItem key={account} value={account}>{account}</SelectItem>)}</SelectContent>
        </Select>
        <p>Agents and terminals in this workspace use this account.</p>
      </div>
      : <p>Using {status.account}.</p>}
    <p>{trackingText(status)}</p>
    {status.error && <p className="flex items-center gap-1.5">
      <Icon name="warning" className="text-destructive" />
      <span>Could not update from GitHub: {status.error.split('\n')[0]}{status.updated > 0 && ` Showing data from ${time(status.updated)}.`}</span>
    </p>}
  </div>;
}
