// A pane of the tabbed area: its tab strip and its active tab's view, the
// agent, file, or diff. A dragged tab drops on the strip, before the
// tab under the pointer, or on the body, where an edge within a quarter of the
// pane splits it.
import {useRef, useState, type DragEvent} from 'react';
import {WorkspaceDashboard} from '../sessions/workspace-dashboard.tsx';
import {DiffView} from '../changes/diff-view.tsx';
import {Card} from '../components/ui/card';
import {FileView} from '../files/file-view.tsx';
import {useStore} from '../store.ts';
import {activatePane, moveTab, tabEntry} from './editor.ts';
import {EditorTabs} from './editor-tabs.tsx';
import {panes} from './tab-layout.ts';
import type {Edge, Pane, Placement} from './types.ts';

function edgeAt(e: DragEvent, body: HTMLElement): Edge {
  if (!(e.target instanceof Node) || !body.contains(e.target)) return 'center';
  const r = body.getBoundingClientRect();
  const x = (e.clientX - r.left) / r.width, y = (e.clientY - r.top) / r.height;
  const sides: [Edge, number][] = [['left', x], ['right', 1 - x], ['top', y], ['bottom', 1 - y]];
  const [edge, distance] = sides.sort((a, b) => a[1] - b[1])[0];
  return distance < 0.25 ? edge : 'center';
}

export function EditorPane({pane, focused, placement}: {pane: Pane; focused: boolean; placement: Placement}) {
  const [state, store] = useStore();
  const [drop, setDrop] = useState<Edge | null>(null);
  const body = useRef<HTMLDivElement>(null);
  const dragging = state.editor?.tabs.dragging;
  const entries = pane.tabs.map(id => tabEntry(state, id));
  const active = tabEntry(state, pane.active);
  const allPanes = panes(state.editor?.tabs.root ?? null);
  const activePane = allPanes.find(item => item.id === state.editor?.activePane) ?? allPanes[0];
  const isActive = focused && activePane.id === pane.id;
  const activate = () => store.update(s => activatePane(s, pane.id));

  // WebKit decides whether a drop is accepted from dragenter alone when the
  // pointer enters a new element, and from dragover only while it stays in
  // one, so both accept the drag.
  const over = (e: DragEvent) => {
    if (!dragging || !body.current) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
    setDrop(edgeAt(e, body.current));
  };
  // dragleave fires as the pointer crosses each child; only leaving the
  // pane's rectangle ends the drop.
  const left = (e: DragEvent<HTMLDivElement>) => {
    const r = e.currentTarget.getBoundingClientRect();
    if (e.clientX < r.left || e.clientX >= r.right || e.clientY < r.top || e.clientY >= r.bottom) setDrop(null);
  };
  const dropped = (e: DragEvent) => {
    if (!dragging || !body.current) return;
    e.preventDefault();
    const edge = edgeAt(e, body.current);
    const target = e.target instanceof Element ? e.target.closest<HTMLElement>('[data-tab]') : null;
    const before = edge === 'center' ? target?.dataset.tab ?? null : null;
    store.update(s => moveTab(s, dragging, pane.id, edge, before));
    setDrop(null);
  };

  return (
    <div className="editor-pane relative flex h-full min-h-0 min-w-0 flex-col" data-pane={pane.id}
      onFocusCapture={activate} onPointerDownCapture={activate} data-drop={drop ?? undefined} onDragEnter={over} onDragOver={over} onDragLeave={left} onDrop={dropped}>
      {drop && <Card aria-hidden="true" className={`pointer-events-none absolute z-10 opacity-50 ${
        drop === 'left' ? 'inset-y-0 left-0 w-1/2' : drop === 'right' ? 'inset-y-0 right-0 w-1/2' :
        drop === 'top' ? 'inset-x-0 top-0 h-1/2' : drop === 'bottom' ? 'inset-x-0 bottom-0 h-1/2' : 'inset-0'}`} />}
      <EditorTabs pane={pane} entries={entries} placement={placement} />
      <div ref={body} className="editor-pane-body relative flex min-h-0 flex-1 flex-col">
        {active.kind === 'dashboard' ? <WorkspaceDashboard />
          : active.kind === 'diff' ? <DiffView key={active.id} id={active.id} path={active.path} active={isActive} />
          : <FileView key={active.id} id={active.id} path={active.path} active={isActive} />}
      </div>
    </div>
  );
}
