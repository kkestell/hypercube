// A pane of the tabbed area: its tab strip and its active tab's view, the
// agent, the browser, or a file. A dragged tab drops on the strip, before the tab under the pointer, or on the
// body, where an edge within a quarter of the pane splits it.
import {useRef, useState} from 'preact/hooks';
import {AgentPane} from '../agent/agent-pane.jsx';
import {BrowserView} from '../browser/browser-view.jsx';
import {shownSession} from '../sessions/selection.js';
import {FileView} from '../files/file-view.jsx';
import {useStore} from '../store.js';
import {moveTab, tabEntry} from './editor.js';
import {EditorTabs} from './editor-tabs.jsx';
import './editor-pane.css';

function edgeAt(e, body) {
  if (!body.contains(e.target)) return 'center';
  const r = body.getBoundingClientRect();
  const x = (e.clientX - r.left) / r.width, y = (e.clientY - r.top) / r.height;
  const [edge, distance] = [['left', x], ['right', 1 - x], ['top', y], ['bottom', 1 - y]].sort((a, b) => a[1] - b[1])[0];
  return distance < 0.25 ? edge : 'center';
}

// toggles puts the layout toggles at the right of the pane's tab strip.
export function EditorPane({pane, toggles}) {
  const [state, store] = useStore();
  const [drop, setDrop] = useState(null);
  const body = useRef(null);
  const shown = shownSession(state);
  const dragging = state.session?.tabs.dragging;
  const entries = pane.tabs.map(id => tabEntry(state, id));
  const active = entries.find(entry => entry.id === pane.active);

  const over = e => {
    if (!dragging) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
    setDrop(edgeAt(e, body.current));
  };
  const left = e => {
    if (!e.currentTarget.contains(e.relatedTarget)) setDrop(null);
  };
  const dropped = e => {
    if (!dragging) return;
    e.preventDefault();
    const edge = edgeAt(e, body.current);
    const before = edge === 'center' ? e.target.closest('[data-tab]')?.dataset.tab ?? null : null;
    store.update(s => moveTab(s, dragging, pane.id, edge, before));
    setDrop(null);
  };

  return (
    <div class="editor-pane" data-drop={drop ?? undefined} onDragOver={over} onDragLeave={left} onDrop={dropped}>
      <EditorTabs pane={pane} entries={entries} toggles={toggles} />
      <div ref={body} class="editor-pane-body">
        {active.agent ? <AgentPane key={shown.id} session={shown} />
          : active.browser ? <BrowserView key={shown.id} session={shown.id} />
          : <FileView key={active.id} id={active.id} path={active.path} />}
      </div>
    </div>
  );
}
