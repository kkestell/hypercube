import {Card} from '../components/ui/card';
// A pane of the tabbed area: its tab strip and its active tab's view, the
// agent, browser, file, or diff. A dragged tab drops on the strip, before the
// tab under the pointer, or on the
// body, where an edge within a quarter of the pane splits it.
import {useRef, useState} from 'react';
import {AgentPane} from '../agent/agent-pane.jsx';
import {BrowserView} from '../browser/browser-view.jsx';
import {shownSession} from '../sessions/selection.js';
import {DiffView} from '../changes/diff-view.jsx';
import {FileView} from '../files/file-view.jsx';
import {useStore} from '../store.js';
import {moveTab, tabEntry} from './editor.js';
import {EditorTabs} from './editor-tabs.jsx';


function edgeAt(e, body) {
  if (!body.contains(e.target)) return 'center';
  const r = body.getBoundingClientRect();
  const x = (e.clientX - r.left) / r.width, y = (e.clientY - r.top) / r.height;
  const [edge, distance] = [['left', x], ['right', 1 - x], ['top', y], ['bottom', 1 - y]].sort((a, b) => a[1] - b[1])[0];
  return distance < 0.25 ? edge : 'center';
}

export function EditorPane({pane}) {
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
    <div className="editor-pane relative flex h-full min-h-0 min-w-0 flex-col" data-pane={pane.id}
      onFocusCapture={() => store.update(s => s.workbench.activePane === pane.id ? s : {...s, workbench: {...s.workbench, activePane: pane.id}})}
      onPointerDownCapture={() => store.update(s => s.workbench.activePane === pane.id ? s : {...s, workbench: {...s.workbench, activePane: pane.id}})} data-drop={drop ?? undefined} onDragOver={over} onDragLeave={left} onDrop={dropped}>
      {drop && <Card aria-hidden="true" className={`pointer-events-none absolute z-10 opacity-50 ${
        drop === 'left' ? 'inset-y-0 left-0 w-1/2' : drop === 'right' ? 'inset-y-0 right-0 w-1/2' :
        drop === 'top' ? 'inset-x-0 top-0 h-1/2' : drop === 'bottom' ? 'inset-x-0 bottom-0 h-1/2' : 'inset-0'}`} />}
      <EditorTabs pane={pane} entries={entries} />
      <div ref={body} className="editor-pane-body relative flex min-h-0 flex-1 flex-col">
        {active.agent ? <AgentPane key={shown.id} session={shown} />
          : active.browser ? <BrowserView key={shown.id} session={shown.id} />
          : active.diff ? <DiffView key={active.id} id={active.id} path={active.path} />
          : <FileView key={active.id} id={active.id} path={active.path} />}
      </div>
    </div>
  );
}
