// The Files tree of the attached session's working directory, as VS Code's
// Explorer: folders list on first expand, a click opens a file in the preview
// tab, and a double-click keeps it open. A folder's children stay drawn while
// it is collapsed, so vscode-tree-item keeps treating it as a folder.
import {useEffect, useRef} from 'preact/hooks';
import {fileTabId, keepOpen, openFileTab} from '../editor/editor.js';
import {useTreeOpen} from '../shared/use-tree-open.js';
import {useStore} from '../store.js';
import {FileIcon} from './file-icon.jsx';
import {loadFolder, setFolderOpen} from './files.js';
import './files-panel.css';

export function FilesPanel() {
  const [state, store] = useStore();
  const files = state.session?.files;
  const tree = useRef(null);

  useTreeOpen(tree, (path, open) => setFolderOpen(store, path, open));
  useEffect(() => {
    if (state.session) loadFolder(store, state.session.id, '');
  }, []);

  const selected = e => {
    const path = e.detail[0]?.dataset.file;
    if (path !== undefined) openFileTab(store, path);
  };

  const entries = (dir, level) => (files?.listings[dir] ?? []).map(entry => {
    const path = dir ? dir + '/' + entry.name : entry.name;
    if (entry.dir) {
      return (
        <vscode-tree-item key={path} data-key={path} style={{'--level': level}} branch open={!!files.expanded[path]}>
          {entry.name}
          {entries(path, level + 1)}
        </vscode-tree-item>
      );
    }
    return (
      <vscode-tree-item key={path} data-file={path} style={{'--level': level}} onDblClick={() => store.update(s => keepOpen(s, fileTabId(path)))}>
        <FileIcon slot="icon-leaf" name={entry.name} />
        {entry.name}
      </vscode-tree-item>
    );
  });

  return <vscode-tree ref={tree} class="files-panel" aria-label="Files" onvsc-tree-select={selected}>{entries('', 0)}</vscode-tree>;
}
