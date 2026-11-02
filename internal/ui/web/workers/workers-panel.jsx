import {Fragment} from 'preact';
import {useEffect, useRef, useState} from 'preact/hooks';
import {AgentPane} from '../agent/agent-pane.jsx';
import {shownAgents} from '../sessions/selection.js';
import {AgentStatus} from '../shared/agent-status.jsx';
import {useStore} from '../store.js';
import {resizeWorkers} from './worker-sizing.js';
import './workers-panel.css';

// age reads as now, 3m, 2h, 1d.
export function age(ms, now = Date.now()) {
  const minutes = Math.floor((now - ms) / 60000);
  if (minutes < 1) return 'now';
  if (minutes < 60) return minutes + 'm';
  return minutes < 1440 ? Math.floor(minutes / 60) + 'h' : Math.floor(minutes / 1440) + 'd';
}

export function WorkersPanel() {
  const [state] = useStore();
  const [expanded, setExpanded] = useState({});
  const [weights, setWeights] = useState({});
  const root = useRef(null);
  const stopDrag = useRef(() => {});
  useEffect(() => () => stopDrag.current(), []);
  const workers = shownAgents(state).filter(a => a.role !== 'lead');
  const expandedIds = workers.filter(worker => expanded[worker.id]).map(worker => worker.id);

  const drag = (event, before, after) => {
    if (event.button !== 0) return;
    event.preventDefault();
    stopDrag.current();
    const heights = Object.fromEntries([...root.current.querySelectorAll('.workers-panel-card-expanded')]
      .map(card => [card.dataset.worker, card.querySelector('.workers-panel-body').getBoundingClientRect().height]));
    const start = event.clientY;
    setWeights(heights);
    const move = e => setWeights(resizeWorkers(heights, before, after, e.clientY - start));
    const stop = () => {
      window.removeEventListener('pointermove', move);
      window.removeEventListener('pointerup', stop);
      window.removeEventListener('pointercancel', stop);
    };
    stopDrag.current = stop;
    window.addEventListener('pointermove', move);
    window.addEventListener('pointerup', stop);
    window.addEventListener('pointercancel', stop);
  };

  if (!workers.length) return <div class="workers-panel"><p class="workers-panel-empty">No workers yet.</p></div>;
  return (
    <div ref={root} class="workers-panel">
      {workers.map(worker => {
        const open = !!expanded[worker.id];
        const index = expandedIds.indexOf(worker.id);
        const after = expandedIds[index + 1];
        return <Fragment key={worker.id}>
          <section data-worker={worker.id} class={open ? 'workers-panel-card workers-panel-card-expanded' : 'workers-panel-card'} style={open ? {flexGrow: weights[worker.id] ?? 1} : undefined}>
            <button type="button" class="workers-panel-header" aria-expanded={open}
              onClick={() => setExpanded({...expanded, [worker.id]: !open})}>
              <vscode-icon name={open ? 'chevron-down' : 'chevron-right'} />
              <span class="workers-panel-name">{worker.name}</span>
              <AgentStatus agent={worker} />
            </button>
            {open ? <div class="workers-panel-body"><AgentPane agent={worker} /></div> : worker.status ? <>
              <p class="workers-panel-status">{worker.status}</p>
              <footer class="workers-panel-age">{age(worker.statusAt)}</footer>
            </> : <p class="workers-panel-empty">No update yet.</p>}
          </section>
          {open && after && <div class="workers-panel-divider" role="separator" aria-label="Resize worker transcripts"
            aria-orientation="horizontal" onPointerDown={event => drag(event, worker.id, after)} />}
        </Fragment>;
      })}
    </div>
  );
}
