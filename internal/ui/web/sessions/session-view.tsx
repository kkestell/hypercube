// The center: the shown session's tabbed area of its agent and open files, or
// a workspace invitation when none exists. While the page is visible, a
// shown session's unseen reply is seen.
import {useEffect} from 'react';
import {Button} from '../components/ui/button';
import {EditorArea} from '../editor/editor-area.tsx';
import {EditorTabs} from '../editor/editor-tabs.tsx';
import {useStore} from '../store.ts';
import {Icon} from '../shared/icon.tsx';
import {addWorkspace, markSeen} from './session-actions.ts';
import {currentSession, shownSession} from './selection.ts';

export function SessionView() {
  const [state, store] = useStore();
  const session = currentSession(state);

  useEffect(() => {
    if (session?.unseen && state.visible) markSeen(store, session.id);
  }, [session?.id, session?.unseen, state.visible]);

  if (shownSession(state)) return <EditorArea />;
  return <>
    <EditorTabs pane={null} entries={[]} placement={{top: true, left: true, right: true}} />
    {state.workspaces.length === 0 && <section aria-label="No workspaces" className="flex min-h-0 flex-1 flex-col items-center justify-center gap-4 p-6 text-center">
      <Icon name="new-folder" className="size-8 text-muted-foreground" />
      <h1 className="text-lg font-medium">Add a workspace to get started</h1>
      <p className="max-w-sm text-sm text-muted-foreground">Choose a project folder, then start a session with a coding agent.</p>
      <Button onClick={() => addWorkspace(store)}><Icon name="new-folder" />Add Workspace</Button>
    </section>}
  </>;
}
