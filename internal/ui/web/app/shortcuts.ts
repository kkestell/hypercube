// The page's keyboard shortcuts: their bindings, which also supply the
// desktop app's menus and the keyboard reference, and what each does.
import {focusComposerField} from '../agent/focus-composer.ts';
import {chooseSession} from '../sessions/session-actions.ts';
import {openUnsent} from '../sessions/selection.ts';
import {sidebarRows} from '../sidebar/sidebar-rows.ts';
import {focusFileSearch, openDialog, openSettings, toggle, toggleWrap} from './workbench.ts';
import type {Store} from '../store.ts';
import {hypercube} from './window.ts';
import {binding, keysLabel, linux} from './keys.ts';
import type {Region} from './types.ts';

export type ShortcutId = 'settings' | 'new' | 'next' | 'previous' | 'select' | 'interrupt' | 'searchSessions'
  | 'searchFiles' | 'quickOpen' | 'composer' | 'sidebar' | 'panel' | 'rightPanel' | 'zoomIn' | 'zoomOut'
  | 'zoomReset' | 'wrap' | 'nextChange' | 'previousChange' | 'find' | 'findNext' | 'findPrevious' | 'help';

export type Section = 'App' | 'Session' | 'View' | 'Help';

// Shortcut is one key binding. menu puts it in the desktop app's menus, which
// then own every key of it, native has the app handle it, and terminal keeps
// it working while a terminal has focus. keys is written for macOS; linux
// replaces it on Linux where its keys collide with the desktop's.
export interface Shortcut {
  id: ShortcutId;
  section: Section;
  title: string;
  keys: string[];
  linux?: string[];
  menu?: boolean;
  native?: boolean;
  terminal?: boolean;
}

// Bindings also supply the desktop app's menus and the keyboard reference.
export const shortcuts: Shortcut[] = [
  {id: 'settings', section: 'App', title: 'Settings…', keys: ['⌘,'], menu: true, terminal: true},
  {id: 'new', section: 'Session', title: 'New Session', keys: ['⌘N'], menu: true},
  {id: 'next', section: 'Session', title: 'Next Session', keys: ['⌘⌥↓', '⌃Tab'], linux: ['⌃PageDown', '⌃Tab'], menu: true},
  {id: 'previous', section: 'Session', title: 'Previous Session', keys: ['⌘⌥↑', '⌃⇧Tab'], linux: ['⌃PageUp', '⌃⇧Tab'], menu: true},
  {id: 'select', section: 'Session', title: 'Select Session', keys: ['⌘1–9']},
  {id: 'interrupt', section: 'Session', title: 'Interrupt Agent', keys: ['Escape']},
  {id: 'searchSessions', section: 'Session', title: 'Search Sessions', keys: ['⌘K']},
  {id: 'searchFiles', section: 'Session', title: 'Search Files', keys: ['⌘⇧F'], terminal: true},
  {id: 'quickOpen', section: 'Session', title: 'Quick Open', keys: ['⌘P'], terminal: true},
  {id: 'composer', section: 'Session', title: 'Focus Composer', keys: ['⌘L']},
  {id: 'sidebar', section: 'View', title: 'Toggle Sidebar', keys: ['⌘B'], menu: true},
  {id: 'panel', section: 'View', title: 'Toggle Terminal', keys: ['⌘J'], menu: true},
  {id: 'rightPanel', section: 'View', title: 'Toggle Right Panel', keys: ['⌘⌥B'], menu: true},
  {id: 'zoomIn', section: 'View', title: 'Zoom In', keys: ['⌘=', '⌘+'], menu: true, native: true},
  {id: 'zoomOut', section: 'View', title: 'Zoom Out', keys: ['⌘-'], menu: true, native: true},
  {id: 'zoomReset', section: 'View', title: 'Actual Size', keys: ['⌘0'], menu: true, native: true},
  {id: 'wrap', section: 'View', title: 'Toggle Word Wrap', keys: ['⌥Z']},
  {id: 'nextChange', section: 'View', title: 'Next Change', keys: ['⌥F5'], linux: ['F7']},
  {id: 'previousChange', section: 'View', title: 'Previous Change', keys: ['⌥⇧F5'], linux: ['⇧F7']},
  {id: 'find', section: 'View', title: 'Find', keys: ['⌘F']},
  {id: 'findNext', section: 'View', title: 'Next Match', keys: ['⌘G', 'F3']},
  {id: 'findPrevious', section: 'View', title: 'Previous Match', keys: ['⌘⇧G', '⇧F3']},
  {id: 'help', section: 'Help', title: 'Keyboard Shortcuts', keys: ['⌘/'], menu: true},
];

// A keystroke, from a DOM or React keyboard event.
interface Keystroke {
  key: string;
  code?: string;
  metaKey?: boolean;
  altKey?: boolean;
  ctrlKey?: boolean;
  shiftKey?: boolean;
}

// keysOf is the notation of the shortcut's bindings on this platform.
export const keysOf = (item: Shortcut) => linux() && item.linux ? item.linux : item.keys;

export function matches(event: Keystroke, id: ShortcutId) {
  return keysOf(shortcutOf(id)).some(text => {
    const b = binding(text);
    const key = event.altKey && event.code && ['KeyB', 'KeyZ'].includes(event.code) ? event.code.slice(3).toLowerCase() : event.key.toLowerCase();
    const range = b.key.match(/^(.)–(.)$/);
    const keyMatches = range ? key.length === 1 && key >= range[1] && key <= range[2] : key === b.key.toLowerCase();
    return keyMatches && !!event.metaKey === b.metaKey && !!event.altKey === b.altKey && !!event.ctrlKey === b.ctrlKey && !!event.shiftKey === b.shiftKey;
  });
}

function shortcutOf(id: ShortcutId): Shortcut {
  const item = shortcuts.find(s => s.id === id);
  if (!item) throw new Error(`No shortcut ${id}.`);
  return item;
}

// firstKeys is the label of a shortcut's first binding.
export const firstKeys = (id: ShortcutId) => keysLabel(keysOf(shortcutOf(id))[0]);

export const shortcutTitle = (id: ShortcutId) => shortcutOf(id).title;

// terminalShortcut says a shortcut marked terminal matches the keystroke, which
// the terminal then leaves to the page and the menus.
export const terminalShortcut = (event: Keystroke) => shortcuts.some(item => item.terminal && matches(event, item.id));

// A region's element carries its name in data-region.
const inRegion = (target: Element, region: Region) => !!target.closest(`[data-region="${region}"]`);

// shortcutAction is what shortcut id does for a keystroke in target, or
// undefined when it does nothing there. editing says the keystroke is in a text
// field, which keeps its own keys.
function shortcutAction(id: ShortcutId, target: Element, editing: boolean, key: string, store: Store): (() => void) | undefined {
  const state = store.get();
  switch (id) {
    case 'wrap':
      return editing ? undefined : () => store.update(toggleWrap);
    case 'searchSessions':
      return () => store.update(s => openDialog(s, 'sessionSearch'));
    case 'searchFiles':
      return state.editor && !state.unsent ? () => store.update(focusFileSearch) : undefined;
    case 'quickOpen':
      return state.editor && !state.unsent ? () => store.update(s => openDialog(s, 'quickOpen')) : undefined;
    case 'new':
      return () => store.update(s => openUnsent(s));
    case 'composer':
      return focusComposerField;
    case 'select': {
      const session = state.workspaces.flatMap(w => w.sessions.filter(s => !s.ended))[Number(key) - 1];
      return session && (() => chooseSession(store, session.id));
    }
    case 'next':
    case 'previous': {
      const rows = sidebarRows(state);
      const index = rows.findIndex(row => row.id === state.selected);
      const previous = id === 'previous';
      const session = rows[index < 0 ? (previous ? rows.length - 1 : 0) : (index + (previous ? -1 : 1) + rows.length) % rows.length]?.id;
      return session ? () => chooseSession(store, session) : undefined;
    }
    case 'sidebar':
    case 'panel':
    case 'rightPanel':
      if (editing) return undefined;
      return () => {
        if ((id === 'panel' ? state.editor?.panel : state.workbench[id]) && inRegion(target, id)) focusComposerField();
        store.update(s => toggle(s, id));
      };
    case 'settings':
      return () => store.update(s => openSettings(s));
    case 'help':
      return () => store.update(s => openDialog(s, 'shortcuts'));
    default:
      return undefined;
  }
}

const editingTarget = (target: Element) => !!target.closest('input, textarea, [contenteditable="true"]');

// shortcut runs the shortcut that a key press in the page names. In the
// desktop app the menus run the shortcuts they own.
export function shortcut(e: KeyboardEvent, store: Store) {
  if (e.defaultPrevented || e.repeat || e.isComposing || !(e.target instanceof Element)) return;
  if (e.target.closest('[role=menu], [role=dialog], [role=alertdialog]')) return;
  if (inRegion(e.target, 'panel') && !terminalShortcut(e)) return;
  if (store.get().confirmation) return;
  const item = shortcuts.find(s => matches(e, s.id));
  if (item?.menu && hypercube()) return;
  const action = item && shortcutAction(item.id, e.target, editingTarget(e.target), e.key, store);
  if (!action) return;
  e.preventDefault();
  e.stopPropagation();
  action();
}

// nativeShortcut runs the shortcut that the app's menu names, for the element
// that has focus.
export function nativeShortcut(id: string, store: Store) {
  const item = shortcuts.find(s => s.id === id);
  if (item) shortcutAction(item.id, document.activeElement ?? document.body, false, '', store)?.();
}
