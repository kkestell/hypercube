// The menu items that ask the desktop app to open folders and files in other
// applications. A page outside the app has no such items.
import {showError} from '../notifications/errors.ts';
import type {ContextMenuItem} from '../shared/context-menu.tsx';
import type {Store} from '../store.ts';
import {hypercube} from './window.ts';

type App = 'vscode' | 'folder' | 'reveal' | 'terminal';

const mac = () => hypercube()?.platform === 'darwin';
const fileManager = () => mac() ? 'Finder' : 'Files';

const open = (store: Store, app: App, path: string) => async () => {
  const error = await hypercube()?.open(app, path);
  if (error) showError(store, error);
};

// openItems are the menus' group for opening the directory dir.
export function openItems(store: Store, dir: string): ContextMenuItem[] {
  if (!hypercube()) return [];
  return [
    {label: 'Open in VS Code', icon: 'code', run: open(store, 'vscode', dir)},
    {label: mac() ? 'Reveal in Finder' : 'Show in Files', icon: 'folder-open', run: open(store, 'folder', dir)},
    ...(mac() ? [{label: 'Open in Terminal', icon: 'terminal' as const, run: open(store, 'terminal', dir)}] : []),
  ];
}

// revealItem shows the file or folder at path in the file manager.
export const revealItem = (store: Store, path: string): ContextMenuItem[] =>
  hypercube() ? [{label: `Show in ${fileManager()}`, icon: 'folder-open', run: open(store, 'reveal', path)}] : [];
