import {Button} from '../components/ui/button';
import {selectedWorkspace} from '../editor/target.ts';
import {useStore} from '../store.ts';
import {chooseSession} from './session-actions.ts';
import {statusOf} from './session-status.tsx';

export function WorkspaceDashboard() {
  const [state, store] = useStore();
  const workspace = selectedWorkspace(state);
  if (!workspace) return null;
  const sessions = workspace.sessions.slice(0, 10);
  return <section aria-label="Workspace dashboard" className="min-h-0 flex-1 overflow-auto p-6">
    <div className="mx-auto max-w-3xl">
      <h1 className="mb-6 text-xl font-medium">{workspace.name}</h1>
      <h2 className="mb-2 text-sm font-medium">Recent Sessions</h2>
      {sessions.length ? <ul className="space-y-1">{sessions.map(session => <li key={session.id}>
        <Button variant="ghost" className="w-full justify-start font-normal" onClick={() => chooseSession(store, session.id)}>
          {statusOf(session, false, session.attention || state.requests.some(request => request.session === session.id), false)}
          <span className="truncate">{session.title}</span>
        </Button>
      </li>)}</ul> : <p className="text-sm text-muted-foreground">No sessions yet.</p>}
    </div>
  </section>;
}
