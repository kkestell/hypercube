// The center: the session's agent and open files in resizable columns. While the
// page is visible, a shown session's unseen reply is seen.
import {useEffect, useState} from 'preact/hooks';
import {AgentPane} from '../agent/agent-pane.jsx';
import {EditorArea} from '../editor/editor-area.jsx';
import {Split} from '../shared/split.jsx';
import {useStore} from '../store.js';
import {Tesseract} from '../tesseract/tesseract.jsx';
import {markSeen} from './session-actions.js';
import {currentSession, shownSession} from './selection.js';

export function SessionView() {
  const [state, store] = useStore();
  const [share, setShare] = useState(50);
  const session = currentSession(state);
  const shown = shownSession(state);
  const hasFiles = !!state.session?.tabs.root;

  useEffect(() => {
    if (session?.unseen && state.visible) markSeen(store, session.id);
  }, [session?.id, session?.unseen, state.visible]);

  if (!shown) return <Tesseract />;
  return (
    <Split size={share} minStart="15%" minEnd="15%" collapsed={hasFiles ? null : 'end'} onResize={setShare}>
      <AgentPane key={shown.id} session={shown} />
      {state.session && <EditorArea />}
    </Split>
  );
}
