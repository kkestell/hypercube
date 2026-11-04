// The tabbed area: the shown session's pane tree, drawn as nested splits
// around panes. An unsent session has only its agent's tab. Only this area
// accepts tab moves and splits.
import {Split} from '../shared/split.jsx';
import {useStore} from '../store.js';
import {agentTabs, resizeSplit} from './editor.js';
import {EditorPane} from './editor-pane.jsx';


function Node({node}) {
  const [, store] = useStore();
  if (node.tabs) return <EditorPane pane={node} />;
  return (
    <Split key={node.id}
      split={node.axis === 'x' ? 'vertical' : 'horizontal'}
      size={node.ratio}
      minStart="15%"
      minEnd="15%"
      onResize={ratio => store.update(s => resizeSplit(s, node.id, ratio))}
    >
      {node.children.map(child => <Node key={child.id} node={child} />)}
    </Split>
  );
}

export function EditorArea() {
  const [state] = useStore();
  const {root} = state.session?.tabs ?? agentTabs;
  return <div className="editor-area h-full min-h-0 min-w-0"><Node node={root} /></div>;
}
