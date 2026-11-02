// The tabbed area: the shown session's pane tree, drawn as nested splits
// around panes, with the layout toggles at the right of the top-right pane's
// tab strip. An unsent session has only its agent's tab. Only this area
// accepts tab moves and splits.
import {Split} from '../shared/split.jsx';
import {useStore} from '../store.js';
import {agentTabs, resizeSplit} from './editor.js';
import {EditorPane} from './editor-pane.jsx';
import {cornerPane} from './tab-layout.js';
import './editor-area.css';

function Node({node, corner}) {
  const [, store] = useStore();
  if (node.tabs) return <EditorPane pane={node} toggles={node.id === corner} />;
  return (
    <Split
      split={node.axis === 'x' ? 'vertical' : 'horizontal'}
      size={node.ratio}
      minStart="15%"
      minEnd="15%"
      onResize={ratio => store.update(s => resizeSplit(s, node.id, ratio))}
    >
      {node.children.map(child => <Node key={child.id} node={child} corner={corner} />)}
    </Split>
  );
}

export function EditorArea() {
  const [state] = useStore();
  const {root} = state.session?.tabs ?? agentTabs;
  return <div class="editor-area"><Node node={root} corner={cornerPane(root).id} /></div>;
}
