import {useEffect, useRef} from 'react';
import {Button} from '../components/ui/button';
import {Collapsible, CollapsibleContent, CollapsibleTrigger} from '../components/ui/collapsible';
import {changeTree, loadChanges, statusLabel} from '../changes/changes.ts';
import {ChangedFile} from '../changes/changed-file.tsx';
import {fileTabId, keepOpen, openDiffTab, openFileTab} from '../editor/editor.ts';
import {useScrollMemory} from '../shared/use-scroll-memory.ts';
import {Chevron} from '../shared/icon.tsx';
import {useTreeFocus, type TreeRow} from '../shared/use-tree-focus.ts';
import {Message} from '../shared/message.tsx';
import {useStore} from '../store.ts';
import {fileRow} from './file-row.ts';
import {listedFolder, loadTree, setFolderOpen, shownFolders, treeEntries} from './files.ts';
import {usePathMenu} from './path-menu.tsx';

export function FilesPanel() {
  const [state, store] = useStore();
  const files = state.editor?.files ?? {listings: {}, listingErrors: {}, expanded: {}, open: {}};
  const {listings, listingErrors, expanded} = files;
  const gitChanges = state.editor?.changes;
  const changes = changeTree(gitChanges ?? null);
  const listing = (dir: string) => treeEntries(files, changes, dir);
  const menu = usePathMenu();
  const tree = useRef<HTMLDivElement>(null);
  const scroller = useRef<HTMLDivElement>(null);
  const ready = (listings[''] !== undefined || listingErrors[''] !== undefined) && shownFolders(files).every(path => !listedFolder(files, path) || listings[path] !== undefined || listingErrors[path] !== undefined);
  useScrollMemory(scroller, 'files-tree', ready && state.workbench.rightPanel && state.workbench.rightTab === 'files');
  const visible: TreeRow[] = [];
  const collect = (dir: string) => {
    for (const entry of listing(dir)) {
      const path = dir ? dir + '/' + entry.name : entry.name;
      visible.push({id: path, expandable: entry.dir, expanded: !!expanded[path], parent: dir || undefined});
      if (entry.dir && expanded[path]) collect(path);
    }
  };
  collect('');
  const {tabbable: active, treeProps, keyDown} = useTreeFocus(tree, visible, null, (path, open) => setFolderOpen(store, path, open));
  useEffect(() => {
    if (!state.editor || !state.workbench.rightPanel || state.workbench.rightTab !== 'files') return;
    loadTree(store, shownFolders(state.editor.files));
    loadChanges(store);
  }, [state.editor?.id, state.workbench.rightPanel, state.workbench.rightTab]);
  const entries = (dir: string, level: number) => listing(dir).map(entry => {
    const path = dir ? dir + '/' + entry.name : entry.name;
    const change = entry.dir ? undefined : changes.files.get(path);
    const status = entry.dir ? changes.folders.get(path) : change?.status;
    const props = {role: 'treeitem', 'data-row': path, 'aria-level': level, tabIndex: active === path ? 0 : -1,
      ...menu.handlers(path), title: status ? `${path}, ${statusLabel(status)}` : path,
      'aria-label': status ? `${entry.name}, ${statusLabel(status)}` : entry.name,
      className: fileRow};
    const label = <ChangedFile name={entry.name} status={status} icon={entry.dir ? <Chevron open={expanded[path]} /> : undefined} />;
    if (entry.dir) return <Collapsible key={path} open={!!expanded[path]} onOpenChange={open => setFolderOpen(store, path, open)}>
      <CollapsibleTrigger asChild><Button variant="ghost" size="sm" {...props} aria-expanded={!!expanded[path]}>
        {label}
      </Button></CollapsibleTrigger>
      <CollapsibleContent role="group" className="pl-4">
        {listingErrors[path] && <Message error className="px-2">{listingErrors[path]}</Message>}
        {entries(path, level + 1)}
      </CollapsibleContent>
    </Collapsible>;
    const deleted = change?.status === 'deleted';
    return <Button key={path} variant="ghost" size="sm" {...props} onClick={() => deleted ? openDiffTab(store, path) : openFileTab(store, path)}
      onDoubleClick={() => { if (!deleted) store.update(s => keepOpen(s, fileTabId(path))); }}>{label}</Button>;
  });
  return <div ref={scroller} className="h-full min-h-0 overflow-auto">
    {gitChanges && 'error' in gitChanges && <Message error className="px-4 pt-1">{gitChanges.error}</Message>}
    <div ref={tree} className="p-2 pt-1" role="tree" aria-label="Files" onKeyDown={keyDown} {...treeProps}>
    {listingErrors[''] && <Message error className="px-2">{listingErrors['']}</Message>}
    {listings[''] === null ? <Message>This folder no longer exists.</Message> : entries('', 1)}
    </div>{menu.element}
  </div>;
}
