// File tabs and workspace dashboards, drawn as nested splits around panes.
// Chat stays outside this area; only editor tabs accept tab moves and splits.
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

export function EditorArea({besideChat = false}: {besideChat?: boolean}) {
  const [state] = useStore();
  const area = useRef<HTMLDivElement>(null);
  const focused = usePaneFocus(area, !besideChat);
  const {root} = state.editor?.tabs ?? emptyTabs;
  const placement = {...whole, left: !besideChat, right: !besideChat};
  return <div ref={area} className="editor-area h-full min-h-0 min-w-0">
    {root ? <PaneTree node={root} focused={focused} placement={placement} /> : <EditorTabs pane={null} entries={[]} placement={placement} />}
  </div>;
}
