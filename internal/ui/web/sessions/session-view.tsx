// The center: a session's or workspace's tabs, or the unsent session's chat.
// With nothing selected it is empty, inviting the user to add a workspace
// when none exists.
import {hypercube} from '../app/window.ts';
import {Button} from '../components/ui/button';
import {ChatPane} from '../agent/chat-pane.tsx';
import {EditorArea} from '../editor/editor-area.tsx';
import {EditorTabs} from '../editor/editor-tabs.tsx';
import {useStore} from '../store.ts';
import {Icon} from '../shared/icon.tsx';
import {addWorkspace} from './session-actions.ts';
import {shownSession} from './selection.ts';

export function SessionView() {
  const [state, store] = useStore();
  if (state.editor) return <EditorArea />;
  const shown = shownSession(state);
  if (shown) return <ChatPane key={shown.id} session={shown} />;
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
