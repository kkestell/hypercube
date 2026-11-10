// The center: a session's or workspace's tabs, the unsent session's chat, or
// the pull requests that need the user when nothing is selected, or a
// workspace invitation when none exists.
import {hypercube} from '../app/window.ts';
import {Button} from '../components/ui/button';
import {ChatPane} from '../agent/chat-pane.tsx';
import {EditorArea} from '../editor/editor-area.tsx';
import {EditorTabs} from '../editor/editor-tabs.tsx';
import {MainTabBar} from '../editor/main-tab-bar.tsx';
import {useStore} from '../store.ts';
import {Icon} from '../shared/icon.tsx';
import {addWorkspace} from './session-actions.ts';
import {AllPullRequests} from '../github/all-pull-requests.tsx';
import {homeShown, shownSession} from './selection.ts';

export function SessionView() {
  const [state, store] = useStore();
  if (state.editor) return <EditorArea />;
  const shown = shownSession(state);
  if (shown) return <ChatPane key={shown.id} session={shown} />;
  if (state.workspaces.length > 0 && state.accounts.length > 0 && homeShown(state)) return <>
    <MainTabBar icon="git-pull-request" title="Pull Requests" />
    <AllPullRequests />
  </>;
  return <>
    <EditorTabs pane={null} entries={[]} placement={{top: true, left: true, right: true}} />
    {state.workspaces.length === 0 && <section aria-label="No workspaces" className="flex min-h-0 flex-1 flex-col items-center justify-center gap-4 p-6 text-center">
      <Icon name="new-folder" className="size-8 text-muted-foreground" />
      <h1 className="text-lg font-semibold">Add a workspace to get started</h1>
      <p className="max-w-sm text-sm text-muted-foreground">Choose a project folder, then start a session with a coding agent.</p>
      {hypercube() && <Button onClick={() => addWorkspace(store)}><Icon name="new-folder" />Add Workspace</Button>}
    </section>}
  </>;
}
