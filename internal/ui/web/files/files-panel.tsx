import {useEffect, useLayoutEffect, useRef, useState, type KeyboardEvent} from 'react';
import {Button} from '../components/ui/button';
import {Collapsible, CollapsibleContent, CollapsibleTrigger} from '../components/ui/collapsible';
import {fileTabId, keepOpen, openFileTab} from '../editor/editor.ts';
import {Icon} from '../shared/icon.tsx';
import {useStore} from '../store.ts';
import {FileIcon} from './file-icon.tsx';
import {loadFolder, setFolderOpen} from './files.ts';
import {usePathMenu} from './path-menu.tsx';

export function FilesPanel() {
  const [state, store] = useStore();
  const listings = state.session?.files.listings ?? {};
  const expanded = state.session?.files.expanded ?? {};
  const menu = usePathMenu();
  const tree = useRef<HTMLDivElement>(null);
  const [focused, setFocused] = useState<string | null>(null);
  const visible: string[] = [];
  const collect = (dir: string) => {
    for (const entry of listings[dir] ?? []) {
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
  useEffect(() => { if (state.session) loadFolder(store, state.session.id, ''); }, []);
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
  const entries = (dir: string, level: number) => (listings[dir] ?? []).map(entry => {
    const path = dir ? dir + '/' + entry.name : entry.name;
    const props = {role: 'treeitem', 'data-path': path, 'aria-level': level, tabIndex: active === path ? 0 : -1,
      ...menu.handlers(path), onFocus: () => setFocused(path), className: 'h-7 w-full justify-start px-2 font-normal'};
    if (entry.dir) return <Collapsible key={path} open={!!expanded[path]} onOpenChange={open => setFolderOpen(store, path, open)}>
      <CollapsibleTrigger asChild><Button variant="ghost" size="sm" {...props} data-dir="" aria-expanded={!!expanded[path]}>
        <Icon name={expanded[path] ? 'chevron-down' : 'chevron-right'} /><span className="truncate">{entry.name}</span>
      </Button></CollapsibleTrigger>
      <CollapsibleContent role="group" className="pl-4">{entries(path, level + 1)}</CollapsibleContent>
    </Collapsible>;
    return <Button key={path} variant="ghost" size="sm" {...props} data-file={path} onClick={() => openFileTab(store, path)}
      onDoubleClick={() => store.update(s => keepOpen(s, fileTabId(path)))}><FileIcon name={entry.name} /><span className="truncate">{entry.name}</span></Button>;
  });
  return <><div ref={tree} className="files-panel p-2" role="tree" aria-label="Files" onKeyDown={keyDown}>
    {listings[''] === null ? <p>This session's folder no longer exists.</p> : entries('', 1)}
  </div>{menu.element}</>;
}
