// The center of the page: the tesseract while no lead is shown, otherwise the
// lead, worker stack, and open files in separate resizable columns. While the
// page is visible, a shown session's unseen reply is seen.
import {useEffect, useState} from 'preact/hooks';
import {AgentPane} from '../agent/agent-pane.jsx';
import {EditorArea} from '../editor/editor-area.jsx';
import {Split} from '../shared/split.jsx';
import {useStore} from '../store.js';
import {WorkersPanel} from '../workers/workers-panel.jsx';
import {Tesseract} from '../tesseract/tesseract.jsx';
import {markSeen} from './session-actions.js';
import {currentSession, shownAgents} from './selection.js';

export function SessionView() {
  const [state, store] = useStore();
  const [leadShare, setLeadShare] = useState(50);
  const [workerShare, setWorkerShare] = useState(50);
  const session = currentSession(state);
  const agents = shownAgents(state);
  const lead = agents.find(a => a.role === 'lead');
  const hasWorkers = agents.some(a => a.role !== 'lead');
  const hasFiles = !!state.session?.tabs.root;

  useEffect(() => {
    if (session?.unseen && state.visible) markSeen(store, session.id);
  }, [session?.id, session?.unseen, state.visible]);

  if (!lead) return <Tesseract />;
  return (
    <Split size={leadShare} minStart="15%" minEnd="15%" collapsed={hasWorkers || hasFiles ? null : 'end'} onResize={setLeadShare}>
      <AgentPane agent={lead} />
      <Split size={workerShare} minStart="15%" minEnd="15%"
        collapsed={!hasWorkers ? 'start' : !hasFiles ? 'end' : null}
        onResize={setWorkerShare}>
        <WorkersPanel key={state.session?.id} />
        {state.session && <EditorArea />}
      </Split>
    </Split>
  );
}
