// What a workspace's dashboard shows about its checkout: the working context
// bar, to which a GitHub workspace adds its repository, the account in use,
// and how far the branch is from its upstream; and why the GitHub data may be
// old.
import {post} from '../api.ts';
import {WorkingContext} from '../agent/working-context.tsx';
import {Select, SelectContent, SelectItem, SelectTrigger, SelectValue} from '../components/ui/select';
import type {Workspace} from '../sessions/types.ts';
import {Icon} from '../shared/icon.tsx';
import {useStore, type Store} from '../store.ts';
import type {GitHubStatus} from './types.ts';
import {commits, failureText, trackingText} from './wording.ts';

// chooseAccount saves the account a workspace's new agents and terminals use.
export const chooseAccount = (store: Store, workspace: string, account: string) =>
  post(store, 'workspace', {action: 'github-account', id: workspace, account});

const accountTitle = 'Agents and terminals in this workspace use this account.';

function Tracking({status}: {status: GitHubStatus}) {
  const {upstream, ahead, behind} = status;
  return <>
    {!upstream && <Icon name="cloud-upload" className="size-3.5" title={trackingText(status)} role="img" />}
    {behind > 0 && <span className="flex items-center gap-0.5" title={`${commits(behind)} behind ${upstream}`}><Icon name="arrow-down" className="size-3.5" />{behind}</span>}
    {ahead > 0 && <span className="flex items-center gap-0.5" title={`${commits(ahead)} ahead of ${upstream}`}><Icon name="arrow-up" className="size-3.5" />{ahead}</span>}
  </>;
}

function Repository({workspace, status}: {workspace: Workspace; status: GitHubStatus}) {
  const [, store] = useStore();
  return <>
    <a href={status.url} target="_blank" rel="noopener noreferrer" className="min-w-0 shrink truncate hover:text-foreground hover:underline">{status.repo}</a>
    {status.choices
      ? <Select value={status.account} onValueChange={account => chooseAccount(store, workspace.id, account)}>
        <SelectTrigger aria-label="GitHub account" title={accountTitle} size="sm" className="h-6 text-xs"><Icon name="user" className="size-3.5" /><SelectValue placeholder="Choose an account" /></SelectTrigger>
        <SelectContent>{status.choices.map(account => <SelectItem key={account} value={account}>{account}</SelectItem>)}</SelectContent>
      </Select>
      : <span className="flex shrink-0 items-center gap-1.5" title={accountTitle}><Icon name="user" className="size-3.5" />{status.account}</span>}
  </>;
}

export function WorkspaceContext({workspace}: {workspace: Workspace}) {
  const status = workspace.github;
  return status
    ? <WorkingContext details={<Repository workspace={workspace} status={status} />} tracking={<Tracking status={status} />} />
    : <WorkingContext />;
}

// GitHubError says why the workspace's GitHub data could not be updated.
export function GitHubError({workspace}: {workspace: Workspace}) {
  const status = workspace.github;
  if (!status?.error) return null;
  return <p className="mb-6 flex items-center gap-1.5 text-sm text-muted-foreground">
    <Icon name="warning" className="text-destructive" />
    <span>{failureText('Could not update from GitHub', status.error, status.updated)}</span>
  </p>;
}
