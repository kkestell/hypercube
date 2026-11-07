import {useCallback, useState, type KeyboardEvent, type MouseEvent} from 'react';
import {revealItem} from '../app/desktop-actions.ts';
import {changeTree} from '../changes/changes.ts';
import {openDiffTab} from '../editor/editor.ts';
import {editorDirectory} from '../editor/target.ts';
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
  const dir = editorDirectory(state) ?? '';
  const relative = (path: string) => path.startsWith(dir + '/') ? path.slice(dir.length + 1) : path;
  const absolute = (path: string) => path.startsWith('/') ? path : `${dir.replace(/\/$/, '')}/${path}`;
  const changed = menu && changeTree(state.editor?.changes ?? null).files.has(menu.path);
  const element = menu && dir && <ContextMenu x={menu.x} y={menu.y} onClose={close} items={[
    ...(changed ? [[{label: 'Open Changes', icon: 'source-control' as const, run: () => openDiffTab(store, menu.path)}]] : []),
    revealItem(store, absolute(menu.path)),
    [
      {label: 'Copy Name', icon: 'copy', run: () => navigator.clipboard.writeText(menu.path.slice(menu.path.lastIndexOf('/') + 1))},
      {label: 'Copy Path', icon: 'copy', run: () => navigator.clipboard.writeText(absolute(menu.path))},
      {label: 'Copy Relative Path', icon: 'copy', run: () => navigator.clipboard.writeText(relative(menu.path))},
    ],
  ]} />;
  return {handlers, element};
}
