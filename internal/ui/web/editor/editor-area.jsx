// The tabbed area: the shown session's pane tree, drawn as nested splits
// around panes. An unsent session has only its agent's tab. Only this area
// accepts tab moves and splits.
import {useEffect, useRef, useState} from 'react';
import {Split} from '../shared/split.jsx';
import {useStore} from '../store.js';
import {agentTabs, resizeSplit} from './editor.js';
import {EditorPane} from './editor-pane.jsx';


function Node({node, focused}) {
  const [, store] = useStore();
  if (node.tabs) return <EditorPane pane={node} focused={focused} />;
  return (
    <Split key={node.id}
      split={node.axis === 'x' ? 'vertical' : 'horizontal'}
      size={node.ratio}
      minStart="15%"
      minEnd="15%"
      onResize={ratio => store.update(s => resizeSplit(s, node.id, ratio))}
    >
      {node.children.map(child => <Node key={child.id} node={child} focused={focused} />)}
    </Split>
  );
}

export function EditorArea() {
  const [state] = useStore();
  const [focused, setFocused] = useState(true);
  const area = useRef(null);
  useEffect(() => {
    const focusIn = event => setFocused(event.target === document.body || area.current.contains(event.target));
    const focusOut = event => { if (!event.relatedTarget) setFocused(true); };
    document.addEventListener('focusin', focusIn);
    document.addEventListener('focusout', focusOut);
    return () => {
      document.removeEventListener('focusin', focusIn);
      document.removeEventListener('focusout', focusOut);
    };
  }, []);
  const {root} = state.session?.tabs ?? agentTabs;
  return <div ref={area} className="editor-area h-full min-h-0 min-w-0">
    <Node node={root} focused={focused} />
  </div>;
}
