import {useEffect, useLayoutEffect, useRef, useState} from 'react';
import {Button} from '../components/ui/button';
import {Collapsible, CollapsibleContent, CollapsibleTrigger} from '../components/ui/collapsible';
import {fileTabId, keepOpen, openFileTab} from '../editor/editor.js';
import {Icon} from '../shared/icon.jsx';
import {useStore} from '../store.js';
import {FileIcon} from './file-icon.jsx';
import {loadFolder, setFolderOpen} from './files.js';
import {usePathMenu} from './path-menu.jsx';

export function FilesPanel() {
  const [state, store] = useStore();
  const files = state.session?.files;
  const menu = usePathMenu();
  const tree = useRef(null);
  const [focused, setFocused] = useState(null);
  const visible = [];
  const collect = dir => {
    for (const entry of files?.listings[dir] ?? []) {
      const path = dir ? dir + '/' + entry.name : entry.name;
      visible.push(path);
      if (entry.dir && files.expanded[path]) collect(path);
    }
  };
  collect('');
  const active = visible.includes(focused) ? focused : visible[0];
  const hadFocus = useRef(false);
  hadFocus.current = !!tree.current?.contains(document.activeElement);
  useLayoutEffect(() => {
    if (hadFocus.current && !tree.current.contains(document.activeElement)) tree.current.querySelector('[tabindex="0"]')?.focus();
  });
  useEffect(() => { if (state.session) loadFolder(store, state.session.id, ''); }, []);
  const keyDown = e => {
    const rows = [...tree.current.querySelectorAll('[role="treeitem"]')].filter(row => !row.closest('[hidden]'));
    const row = e.target.closest('[role="treeitem"]');
    const index = rows.indexOf(row);
    if (index < 0) return;
    let next;
    if (e.key === 'ArrowDown') next = rows[Math.min(index + 1, rows.length - 1)];
    else if (e.key === 'ArrowUp') next = rows[Math.max(index - 1, 0)];
    else if (e.key === 'Home') next = rows[0];
    else if (e.key === 'End') next = rows.at(-1);
    else if (e.key === 'ArrowRight' && row.dataset.dir !== undefined) {
      if (!files.expanded[row.dataset.path]) setFolderOpen(store, row.dataset.path, true);
      else next = rows[index + 1];
    } else if (e.key === 'ArrowLeft') {
      if (row.dataset.dir !== undefined && files.expanded[row.dataset.path]) setFolderOpen(store, row.dataset.path, false);
      else next = rows.find(r => r.dataset.path === row.dataset.path.split('/').slice(0, -1).join('/'));
    } else return;
    e.preventDefault(); next?.focus();
  };
  const entries = (dir, level) => (files?.listings[dir] ?? []).map(entry => {
    const path = dir ? dir + '/' + entry.name : entry.name;
    const props = {role: 'treeitem', 'data-path': path, 'aria-level': level, tabIndex: active === path ? 0 : -1,
      ...menu.handlers(path), onFocus: () => setFocused(path), size: 'sm', className: 'h-7 w-full justify-start px-2'};
    if (entry.dir) return <Collapsible key={path} open={!!files.expanded[path]} onOpenChange={open => setFolderOpen(store, path, open)}>
      <CollapsibleTrigger asChild><Button variant="ghost" {...props} data-dir="" aria-expanded={!!files.expanded[path]}>
        <Icon name={files.expanded[path] ? 'chevron-down' : 'chevron-right'} /><span className="truncate">{entry.name}</span>
      </Button></CollapsibleTrigger>
      <CollapsibleContent role="group" className="pl-4">{entries(path, level + 1)}</CollapsibleContent>
    </Collapsible>;
    return <Button key={path} variant="ghost" {...props} data-file={path} onClick={() => openFileTab(store, path)}
      onDoubleClick={() => store.update(s => keepOpen(s, fileTabId(path)))}><FileIcon name={entry.name} /><span className="truncate">{entry.name}</span></Button>;
  });
  return <><div ref={tree} className="files-panel p-2" role="tree" aria-label="Files" onKeyDown={keyDown}>
    {files?.listings[''] === null ? <p>This session's folder no longer exists.</p> : entries('', 1)}
  </div>{menu.element}</>;
}
