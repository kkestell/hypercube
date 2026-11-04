// The center: the shown session's tabbed area of its agent and open files, or
// an empty tab strip when no session shows. While the page is visible, a
// shown session's unseen reply is seen.
import {useEffect} from 'react';
import {EditorArea} from '../editor/editor-area.tsx';
import {EditorTabs} from '../editor/editor-tabs.tsx';
import {useStore} from '../store.ts';
import {markSeen} from './session-actions.ts';
import {currentSession, shownSession} from './selection.ts';

export function SessionView() {
  const [state, store] = useStore();
  const session = currentSession(state);

  useEffect(() => {
    if (session?.unseen && state.visible) markSeen(store, session.id);
  }, [session?.id, session?.unseen, state.visible]);

  if (shownSession(state)) return <EditorArea />;
  return <EditorTabs pane={null} entries={[]} placement={{top: true, left: true, right: true}} />;
}
