import {useCallback, useState} from 'react';
import {currentSession} from '../sessions/selection.js';
import {ContextMenu} from '../shared/context-menu.jsx';
import {useStore} from '../store.js';

export function usePathMenu() {
  const [state] = useStore();
  const [menu, setMenu] = useState(null);
  const close = useCallback(() => setMenu(null), []);
  const open = (event, path) => {
    event.preventDefault();
    const keyboard = event.type === 'keydown';
    const rect = keyboard && event.currentTarget.getBoundingClientRect();
    setMenu({path, x: keyboard ? rect.left : event.clientX, y: keyboard ? rect.bottom : event.clientY});
  };
  const handlers = path => ({
    onContextMenu: event => open(event, path),
    onKeyDown: event => {
      if (event.key === 'ContextMenu' || (event.shiftKey && event.key === 'F10')) open(event, path);
    },
  });
  const dir = currentSession(state)?.dir ?? '';
  const relative = menu?.path.startsWith('/') && menu.path.startsWith(dir + '/') ? menu.path.slice(dir.length + 1) : menu?.path;
  const absolute = menu?.path.startsWith('/') ? menu.path : `${dir.replace(/\/$/, '')}/${menu?.path}`;
  const element = menu && <ContextMenu x={menu.x} y={menu.y} onClose={close} items={[
    {label: 'Copy Path', run: () => navigator.clipboard.writeText(absolute)},
    {label: 'Copy Relative Path', run: () => navigator.clipboard.writeText(relative)},
  ]} />;
  return {handlers, element};
}
