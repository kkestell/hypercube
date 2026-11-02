// The editor area: the attached session's pane tree, drawn as nested splits
// around panes. Only this area accepts tab moves and splits.
import {Split} from '../shared/split.jsx';
import {useStore} from '../store.js';
import {resizeSplit} from './editor.js';
import {EditorPane} from './editor-pane.jsx';
import './editor-area.css';

function Node({node}) {
  const [, store] = useStore();
  if (node.tabs) return <EditorPane pane={node} />;
  return (
    <Split
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
  const root = state.session.tabs.root;
  return <div class="editor-area">{root && <Node node={root} />}</div>;
}
