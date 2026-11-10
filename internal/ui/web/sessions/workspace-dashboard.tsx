import {GitHubError, WorkspaceContext} from '../github/github-header.tsx';
import {PullRequests} from '../github/pull-requests.tsx';
import {Button} from '../components/ui/button';
import {MainTabBar} from '../editor/main-tab-bar.tsx';
import {selectedWorkspace} from '../editor/target.ts';
import {useStore} from '../store.ts';
import {Message} from '../shared/message.tsx';
import {StatusDot} from '../shared/status-dot.tsx';
import {chooseSession} from './session-actions.ts';

const archivedText = (ms: number) => new Date(ms).toLocaleString([], {dateStyle: 'medium', timeStyle: 'short'});

export function WorkspaceDashboard() {
  const [state, store] = useStore();
  const workspace = selectedWorkspace(state);
  if (!workspace) return null;
  const archived = workspace.sessions.filter(session => session.ended)
    .sort((a, b) => (b.archived ?? 0) - (a.archived ?? 0)).slice(0, 10);
  return <section aria-label="Workspace dashboard" className="@container flex h-full min-h-0 min-w-0 flex-col">
    <MainTabBar icon="files" title={workspace.name} />
    <WorkspaceContext workspace={workspace} />
    <div className="min-h-0 flex-1 overflow-auto p-6">
      <GitHubError failure={workspace.github} />
      <div className="grid gap-10 @4xl:grid-cols-[minmax(0,1fr)_20rem]">
        <PullRequests workspace={workspace} />
        <section>
          <h2 className="mb-3 text-lg font-semibold">Archived Sessions</h2>
          {archived.length ? <ul aria-label="Archived sessions" className="space-y-1">{archived.map(session => <li key={session.id}>
            <Button variant="ghost" className="-ml-2 w-full justify-start px-2 font-normal" onClick={() => chooseSession(store, session.id)}>
              <StatusDot kind="ended" title="Ended" />
              <span className="flex-1 truncate text-left">{session.title}</span>
              {session.archived && <time dateTime={new Date(session.archived).toISOString()} className="shrink-0 text-xs text-muted-foreground">{archivedText(session.archived)}</time>}
            </Button>
          </li>)}</ul> : <Message>No archived sessions.</Message>}
        </section>
      </div>
    </div>
  </section>;
}
