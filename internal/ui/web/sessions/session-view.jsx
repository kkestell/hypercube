// The center of the page: the tesseract while nothing is shown, otherwise the
// lead in its own column beside the editor area, which shows while any tab is
// open. While the page is visible, a shown session's unseen reply is seen.
import {useEffect, useState} from 'preact/hooks';
import {AgentPane} from '../agent/agent-pane.jsx';
import {EditorArea} from '../editor/editor-area.jsx';
import {Split} from '../shared/split.jsx';
import {useStore} from '../store.js';
import {Tesseract} from '../tesseract/tesseract.jsx';
import {markSeen} from './session-actions.js';
import {currentSession, shownAgents} from './selection.js';

export function SessionView() {
  const [state, store] = useStore();
  const [leadShare, setLeadShare] = useState(50);
  const session = currentSession(state);
  const agents = shownAgents(state);
  const lead = agents.find(a => a.role === 'lead');

  useEffect(() => {
    if (session?.unseen && state.visible) markSeen(store, session.id);
  }, [session?.id, session?.unseen, state.visible]);

  if (!session && !state.unsent) return <Tesseract />;
  if (!lead) return null;
  return (
    <Split size={leadShare} minStart="15%" minEnd="15%" collapsed={state.session?.tabs.root ? null : 'end'} onResize={setLeadShare}>
      <AgentPane agent={lead} />
      {state.session && <EditorArea agents={agents} />}
    </Split>
  );
}
