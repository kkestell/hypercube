import {useEffect, useLayoutEffect, useRef, useState, type KeyboardEvent} from 'react';
import {Button} from '../components/ui/button';
import {Collapsible, CollapsibleContent, CollapsibleTrigger} from '../components/ui/collapsible';
import {changeTree, loadChanges, statusColor, statusLabel} from '../changes/changes.ts';
import {fileTabId, keepOpen, openDiffTab, openFileTab} from '../editor/editor.ts';
import {useScrollMemory} from '../editor/use-scroll-memory.ts';
import {Chevron} from '../shared/icon.tsx';
import {useStore} from '../store.ts';
import {FileIcon} from './file-icon.tsx';
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
  const [focused, setFocused] = useState<string | null>(null);
  const visible: string[] = [];
  const collect = (dir: string) => {
    for (const entry of listing(dir)) {
      const path = dir ? dir + '/' + entry.name : entry.name;
      visible.push(path);
      if (entry.dir && expanded[path]) collect(path);
    }
  };
  collect('');
  const active = visible.find(path => path === focused) ?? visible[0];
  const hadFocus = useRef(false);
  hadFocus.current = !!tree.current?.contains(document.activeElement);
  useLayoutEffect(() => {
    const root = tree.current;
    if (hadFocus.current && root && !root.contains(document.activeElement)) root.querySelector<HTMLElement>('[tabindex="0"]')?.focus();
  });
  useEffect(() => {
    if (!state.editor || !state.workbench.rightPanel || state.workbench.rightTab !== 'files') return;
    loadTree(store, shownFolders(state.editor.files));
    loadChanges(store);
  }, [state.editor?.id, state.workbench.rightPanel, state.workbench.rightTab]);
  const keyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    if (!(e.target instanceof Element)) return;
    const row = e.target.closest<HTMLElement>('[role="treeitem"]');
    const rows = [...e.currentTarget.querySelectorAll<HTMLElement>('[role="treeitem"]')].filter(row => !row.closest('[hidden]'));
    const index = row ? rows.indexOf(row) : -1;
    if (!row || index < 0) return;
    const path = row.dataset.path ?? '';
    let next: HTMLElement | undefined;
    if (e.key === 'ArrowDown') next = rows[Math.min(index + 1, rows.length - 1)];
    else if (e.key === 'ArrowUp') next = rows[Math.max(index - 1, 0)];
    else if (e.key === 'Home') next = rows[0];
    else if (e.key === 'End') next = rows.at(-1);
    else if (e.key === 'ArrowRight' && row.dataset.dir !== undefined) {
      if (!expanded[path]) setFolderOpen(store, path, true);
      else next = rows[index + 1];
    } else if (e.key === 'ArrowLeft') {
      if (row.dataset.dir !== undefined && expanded[path]) setFolderOpen(store, path, false);
      else next = rows.find(r => r.dataset.path === path.split('/').slice(0, -1).join('/'));
    } else return;
    e.preventDefault(); next?.focus();
  };
  const entries = (dir: string, level: number) => listing(dir).map(entry => {
    const path = dir ? dir + '/' + entry.name : entry.name;
    const change = entry.dir ? undefined : changes.files.get(path);
    const status = entry.dir ? changes.folders.get(path) : change?.status;
    const color = status ? statusColor[status] : '';
    const props = {role: 'treeitem', 'data-path': path, 'aria-level': level, tabIndex: active === path ? 0 : -1,
      ...menu.handlers(path), onFocus: () => setFocused(path), title: status && `${path} • ${statusLabel(status)}`,
      className: 'h-7 min-w-0 justify-start px-2 font-normal'};
    const label = <span className={`flex min-w-0 items-center gap-1.5 ${color}`}>
      {entry.dir ? <Chevron open={expanded[path]} /> : <FileIcon name={entry.name} />}
      <span className={`truncate ${status === 'deleted' ? 'line-through' : ''}`}>{entry.name}</span>
      {status && <span className="sr-only">, {statusLabel(status)}</span>}
    </span>;
    if (entry.dir) return <Collapsible key={path} open={!!expanded[path]} onOpenChange={open => setFolderOpen(store, path, open)}>
      <CollapsibleTrigger asChild><Button variant="ghost" size="sm" {...props} data-dir="" aria-expanded={!!expanded[path]} className={`${props.className} w-full`}>
        {label}
      </Button></CollapsibleTrigger>
      <CollapsibleContent role="group" className="pl-4">{expanded[path] && <>
        {listingErrors[path] && <p className="px-2 text-sm text-destructive">{listingErrors[path]}</p>}
        {entries(path, level + 1)}
      </>}</CollapsibleContent>
    </Collapsible>;
    const deleted = change?.status === 'deleted';
    return <Button key={path} variant="ghost" size="sm" {...props} className={`${props.className} w-full`} data-file={path} onClick={() => deleted ? openDiffTab(store, path) : openFileTab(store, path)}
      onDoubleClick={() => { if (!deleted) store.update(s => keepOpen(s, fileTabId(path))); }}>{label}</Button>;
  });
  return <div ref={scroller} className="h-full min-h-0 overflow-auto">
    {gitChanges && 'error' in gitChanges && <p className="px-4 pt-1 text-sm text-muted-foreground">{gitChanges.error}</p>}
    <div ref={tree} className="files-panel p-2 pt-1" role="tree" aria-label="Files" onKeyDown={keyDown}>
    {listingErrors[''] && <p className="px-2 text-sm text-destructive">{listingErrors['']}</p>}
    {listings[''] === null ? <p>This folder no longer exists.</p> : entries('', 1)}
    </div>{menu.element}
  </div>;
}
