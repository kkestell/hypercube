import {useCallback, useState, type KeyboardEvent, type MouseEvent} from 'react';
import {post} from '../api.ts';
import {changeTree} from '../changes/changes.ts';
import {openDiffTab} from '../editor/editor.ts';
import {currentSession} from '../sessions/selection.ts';
import {ContextMenu} from '../shared/context-menu.tsx';
import {useStore} from '../store.ts';

interface PathMenu {
  path: string;
  x: number;
  y: number;
}

export function usePathMenu() {
  const [state, store] = useStore();
  const [menu, setMenu] = useState<PathMenu | null>(null);
  const close = useCallback(() => setMenu(null), []);
  const handlers = (path: string) => ({
    onContextMenu: (event: MouseEvent<HTMLElement>) => {
      event.preventDefault();
      setMenu({path, x: event.clientX, y: event.clientY});
    },
    onKeyDown: (event: KeyboardEvent<HTMLElement>) => {
      if (event.key !== 'ContextMenu' && !(event.shiftKey && event.key === 'F10')) return;
      event.preventDefault();
      const rect = event.currentTarget.getBoundingClientRect();
      setMenu({path, x: rect.left, y: rect.bottom});
    },
  });
  const session = currentSession(state);
  const dir = session?.dir ?? '';
  const relative = (path: string) => path.startsWith(dir + '/') ? path.slice(dir.length + 1) : path;
  const absolute = (path: string) => path.startsWith('/') ? path : `${dir.replace(/\/$/, '')}/${path}`;
  const changed = menu && changeTree(state.session?.changes ?? null).files.has(menu.path);
  const element = menu && session && <ContextMenu x={menu.x} y={menu.y} onClose={close} items={[
    ...(changed ? [[{label: 'Open Changes', icon: 'source-control' as const, run: () => openDiffTab(store, menu.path)}]] : []),
    [{label: 'Show in Finder', icon: 'folder-open', run: () => post(store, 'open', {kind: 'path', id: session.id, app: 'finder', path: menu.path})}],
    [
      {label: 'Copy Name', icon: 'copy', run: () => navigator.clipboard.writeText(menu.path.slice(menu.path.lastIndexOf('/') + 1))},
      {label: 'Copy Path', icon: 'copy', run: () => navigator.clipboard.writeText(absolute(menu.path))},
      {label: 'Copy Relative Path', icon: 'copy', run: () => navigator.clipboard.writeText(relative(menu.path))},
    ],
  ]} />;
  return {handlers, element};
}
