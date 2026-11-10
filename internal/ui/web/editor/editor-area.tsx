// A session's or workspace's tabs, its main tab with its file and diff tabs,
// drawn as nested splits around panes.
import {useRef} from 'react';
import {Split} from '../shared/split.tsx';
import {usePaneFocus} from '../shared/use-pane-focus.ts';
import {useStore} from '../store.ts';
import {emptyTabs, resizeSplit} from './editor.ts';
import {EditorPane} from './editor-pane.tsx';
import {EditorTabs} from './editor-tabs.tsx';
import type {LayoutNode, Placement} from './types.ts';

const whole: Placement = {top: true, left: true, right: true};

function PaneTree({node, focused, placement}: {node: LayoutNode; focused: boolean; placement: Placement}) {
  const [, store] = useStore();
  if ('tabs' in node) return <EditorPane pane={node} focused={focused} placement={placement} />;
  const [first, second] = node.children;
  return (
    <Split key={node.id}
      split={node.axis === 'x' ? 'vertical' : 'horizontal'}
      size={node.ratio}
      minStart="15%"
      minEnd="15%"
      onResize={ratio => store.update(s => resizeSplit(s, node.id, ratio))}
    >
      <PaneTree key={first.id} node={first} focused={focused} placement={node.axis === 'x' ? {...placement, right: false} : placement} />
      <PaneTree key={second.id} node={second} focused={focused} placement={node.axis === 'x' ? {...placement, left: false} : {...placement, top: false}} />
    </Split>
  );
}

export function EditorArea() {
  const [state] = useStore();
  const area = useRef<HTMLDivElement>(null);
  const focused = usePaneFocus(area);
  const {root} = state.editor?.tabs ?? emptyTabs;
  return <div ref={area} className="h-full min-h-0 min-w-0" data-editor-area="">
    {root ? <PaneTree node={root} focused={focused} placement={whole} /> : <EditorTabs pane={null} entries={[]} placement={whole} />}
  </div>;
}
