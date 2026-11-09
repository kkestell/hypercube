// The center: permanent chat beside file tabs, a workspace's editor area, or
// the pull requests that need the user when nothing is selected, or a
// workspace invitation when none exists. While the page is visible, a
// shown session's unseen reply is seen.
import {useEffect} from 'react';
import {hypercube} from '../app/window.ts';
import {Button} from '../components/ui/button';
import {ChatPane} from '../agent/chat-pane.tsx';
import {resizeRegion} from '../app/workbench.ts';
import {EditorArea} from '../editor/editor-area.tsx';
import {EditorTabs} from '../editor/editor-tabs.tsx';
import {useStore} from '../store.ts';
import {Icon} from '../shared/icon.tsx';
import {Split} from '../shared/split.tsx';
import {addWorkspace, markSeen} from './session-actions.ts';
import {AllPullRequests} from '../github/all-pull-requests.tsx';
import {currentSession, homeShown, shownSession} from './selection.ts';

export function SessionView() {
  const [state, store] = useStore();
  const session = currentSession(state);

  useEffect(() => {
    if (session?.unseen && state.visible) markSeen(store, session.id);
  }, [session?.id, session?.unseen, state.visible]);

  const shown = shownSession(state);
  if (shown) {
    const hasFiles = !!state.editor?.tabs.root;
    return <Split fixed="end" size={state.workbench.sizes.editor} minStart="20%" minEnd="20%"
      collapsed={hasFiles ? null : 'end'} onResize={size => store.update(s => resizeRegion(s, 'editor', size))}>
      <ChatPane key={shown.id} session={shown} />
      {hasFiles ? <EditorArea besideChat /> : null}
    </Split>;
  }
  if (state.editor) return <EditorArea />;
  if (state.workspaces.length > 0 && homeShown(state)) return <>
    <EditorTabs pane={null} entries={[]} placement={{top: true, left: true, right: true}} />
    <AllPullRequests />
  </>;
  return <>
    <EditorTabs pane={null} entries={[]} placement={{top: true, left: true, right: true}} />
    {state.workspaces.length === 0 && <section aria-label="No workspaces" className="flex min-h-0 flex-1 flex-col items-center justify-center gap-4 p-6 text-center">
      <Icon name="new-folder" className="size-8 text-muted-foreground" />
      <h1 className="text-lg font-medium">Add a workspace to get started</h1>
      <p className="max-w-sm text-sm text-muted-foreground">Choose a project folder, then start a session with a coding agent.</p>
      {hypercube() && <Button onClick={() => addWorkspace(store)}><Icon name="new-folder" />Add Workspace</Button>}
    </section>}
  </>;
}
