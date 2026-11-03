// Two panes with a draggable divider. A fixed pane keeps its size in pixels
// when the split resizes; otherwise the start pane keeps its share.
//
// collapsed names a pane to hide. Both panes stay mounted, so a hidden view
// keeps its scroll, terminal, and drafts: the divider moves to the pane's edge
// and its handle shrinks to nothing. Showing the pane again restores the size
// it had.
import {toChildArray} from 'preact';
import {useLayoutEffect, useRef} from 'preact/hooks';
import './split.css';

const handleSize = 4;

// The control exposes no CSS parts. Give its shadow-root handles native
// split-pane cursors and immediate hover and drag feedback.
const handleStyles = new CSSStyleSheet();
handleStyles.replaceSync(`
  .handle.split-vertical, .handle-overlay.split-vertical { cursor: col-resize; }
  .handle.split-horizontal, .handle-overlay.split-horizontal { cursor: row-resize; }
  .handle.hover, .handle.hide { transition: none; }
  .handle-overlay.active + .handle { background-color: var(--vscode-sash-hoverBorder); }
`);

export function Split({split = 'vertical', fixed = 'none', size, minStart, minEnd, collapsed = null, onResize, children}) {
  const [start, end] = toChildArray(children);
  const ref = useRef(null);
  // The fixed pane's size in pixels, or the start pane's percentage.
  const kept = useRef(size);
  if (fixed === 'none') kept.current = size;
  const length = () => split === 'vertical' ? ref.current.clientWidth : ref.current.clientHeight;

  useLayoutEffect(() => {
    const root = ref.current.shadowRoot;
    if (root) root.adoptedStyleSheets = [...root.adoptedStyleSheets, handleStyles];
  }, []);

  useLayoutEffect(() => {
    const element = ref.current;
    if (collapsed) element.handlePosition = collapsed === 'start' ? '0px' : '100%';
    else if (fixed === 'start') element.handlePosition = `${kept.current}px`;
    // The split has no width until it and the splits around it have rendered.
    else if (fixed === 'end') Promise.resolve(element.updateComplete).then(() => element.handlePosition = `${length() - kept.current}px`);
    else element.handlePosition = `${kept.current}%`;
  }, [collapsed, size]);

  const startDrag = e => {
    if (e.button !== 0 || collapsed) return;
    const element = ref.current;
    const handle = element.shadowRoot?.querySelector('.handle');
    if (!handle || !e.composedPath().includes(handle)) return;
    // Percentage splits cache their dimensions. A parent resize can leave
    // those stale; synchronize to the visible divider before the control grabs it.
    const bounds = element.getBoundingClientRect();
    const rect = handle.getBoundingClientRect();
    const position = split === 'vertical' ? rect.left + rect.width / 2 - bounds.left : rect.top + rect.height / 2 - bounds.top;
    element.handlePosition = `${position}px`;
    handle.setPointerCapture(e.pointerId);
    window.getSelection()?.removeAllRanges();
  };

  const resized = e => {
    const {position, positionInPercentage} = e.detail;
    if (fixed === 'start') kept.current = position;
    else if (fixed === 'end') kept.current = length() - position;
    else onResize?.(positionInPercentage);
  };

  return (
    <vscode-split-layout
      ref={ref}
      class={collapsed ? 'split split-collapsed' : 'split'}
      split={split}
      // A zero-width parent makes percentage panes both grow from 0%.
      // Fix the collapsed side at zero so it stays hidden when its parent opens.
      fixed-pane={collapsed ?? fixed}
      handle-size={collapsed ? 0 : handleSize}
      min-start={collapsed ? undefined : minStart}
      min-end={collapsed ? undefined : minEnd}
      onPointerDownCapture={startDrag}
      onvsc-split-layout-change={resized}
    >
      <div slot="start" class="split-pane">{start}</div>
      <div slot="end" class="split-pane">{end}</div>
    </vscode-split-layout>
  );
}
