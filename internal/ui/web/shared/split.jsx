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

export function Split({split = 'vertical', fixed = 'none', size, minStart, minEnd, collapsed = null, onResize, children}) {
  const [start, end] = toChildArray(children);
  const ref = useRef(null);
  // The fixed pane's size in pixels, or the start pane's percentage.
  const kept = useRef(size);
  if (fixed === 'none') kept.current = size;
  const length = () => split === 'vertical' ? ref.current.clientWidth : ref.current.clientHeight;

  useLayoutEffect(() => {
    const element = ref.current;
    if (collapsed) element.handlePosition = collapsed === 'start' ? '0px' : '100%';
    else if (fixed === 'start') element.handlePosition = `${kept.current}px`;
    else if (fixed === 'end') element.handlePosition = `${length() - kept.current}px`;
    else element.handlePosition = `${kept.current}%`;
  }, [collapsed, size]);

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
      fixed-pane={fixed}
      handle-size={collapsed ? 0 : handleSize}
      min-start={collapsed ? undefined : minStart}
      min-end={collapsed ? undefined : minEnd}
      onvsc-split-layout-change={resized}
    >
      <div slot="start" class="split-pane">{start}</div>
      <div slot="end" class="split-pane">{end}</div>
    </vscode-split-layout>
  );
}
