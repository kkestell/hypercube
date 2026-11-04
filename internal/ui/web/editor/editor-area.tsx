// The tabbed area: the shown session's pane tree, drawn as nested splits
// around panes. An unsent session has only its agent's tab. Only this area
// accepts tab moves and splits.
import {useEffect, useRef, useState} from 'react';
import {Split} from '../shared/split.tsx';
import {useStore} from '../store.ts';
import {agentTabs, resizeSplit} from './editor.ts';
import {EditorPane} from './editor-pane.tsx';
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
  const [focused, setFocused] = useState(true);
  const area = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const focusIn = (event: FocusEvent) => {
      const target = event.target;
      setFocused(target === document.body || (target instanceof Node && !!area.current?.contains(target)));
    };
    const focusOut = (event: FocusEvent) => { if (!event.relatedTarget) setFocused(true); };
    document.addEventListener('focusin', focusIn);
    document.addEventListener('focusout', focusOut);
    return () => {
      document.removeEventListener('focusin', focusIn);
      document.removeEventListener('focusout', focusOut);
    };
  }, []);
  const {root} = state.session?.tabs ?? agentTabs;
  return <div ref={area} className="editor-area h-full min-h-0 min-w-0">
    {root && <PaneTree node={root} focused={focused} placement={whole} />}
  </div>;
}
