// A card per worker of the shown session: its name, indicator, latest report
// and its age, and a button that opens its transcript in the editor area.
import {openWorkerTab} from '../editor/editor.js';
import {shownAgents} from '../sessions/selection.js';
import {AgentStatus} from '../shared/agent-status.jsx';
import {useStore} from '../store.js';
import './workers-panel.css';

// age reads as now, 3m, 2h, 1d.
export function age(ms, now = Date.now()) {
  const minutes = Math.floor((now - ms) / 60000);
  if (minutes < 1) return 'now';
  if (minutes < 60) return minutes + 'm';
  return minutes < 1440 ? Math.floor(minutes / 60) + 'h' : Math.floor(minutes / 1440) + 'd';
}

export function WorkersPanel() {
  const [state, store] = useStore();
  const workers = shownAgents(state).filter(a => a.role !== 'lead');
  const openTranscript = id => {
    store.update(s => openWorkerTab(s, id));
    requestAnimationFrame(() => document.querySelector(`.composer[data-agent="${id}"] textarea`)?.focus());
  };
  if (!workers.length) return <div class="workers-panel"><p class="workers-panel-empty">No workers yet.</p></div>;
  return (
    <div class="workers-panel">
      {workers.map(worker => (
        <article key={worker.id} class={worker.attention ? 'workers-panel-card workers-panel-card-attention' : 'workers-panel-card'}>
          <header class="workers-panel-header">
            <span class="workers-panel-name">{worker.name}</span>
            <AgentStatus agent={worker} />
            <vscode-toolbar-button icon="layout-sidebar-left" label="Open Transcript" title="Open Transcript"
              onClick={() => openTranscript(worker.id)} />
          </header>
          {worker.status ? (
            <>
              <p class="workers-panel-status">{worker.status}</p>
              <footer class="workers-panel-age">{age(worker.statusAt)}</footer>
            </>
          ) : <p class="workers-panel-empty">No update yet.</p>}
        </article>
      ))}
    </div>
  );
}
