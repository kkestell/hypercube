// The center: the shown session's tabbed area of its agent and open files, or
// the tesseract under an empty tab strip with the layout toggles when no session shows. While the page
// is visible, a shown session's unseen reply is seen.
import {useEffect} from 'preact/hooks';
import {EditorArea} from '../editor/editor-area.jsx';
import {EditorTabs} from '../editor/editor-tabs.jsx';
import {useStore} from '../store.js';
import {Tesseract} from '../tesseract/tesseract.jsx';
import {markSeen} from './session-actions.js';
import {currentSession, shownSession} from './selection.js';

export function SessionView() {
  const [state, store] = useStore();
  const session = currentSession(state);

  useEffect(() => {
    if (session?.unseen && state.visible) markSeen(store, session.id);
  }, [session?.id, session?.unseen, state.visible]);

  if (shownSession(state)) return <EditorArea />;
  return (
    <>
      <EditorTabs pane={null} entries={[]} toggles />
      <Tesseract />
    </>
  );
}
