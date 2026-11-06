import {chooseSession} from '../sessions/session-actions.ts';
import {openUnsent} from '../sessions/selection.ts';
import {toggle, toggleWrap} from './workbench.ts';
import type {Store} from '../store.ts';
import {hypercube} from './window.ts';
import type {Region} from './types.ts';

export type ShortcutId = 'settings' | 'new' | 'next' | 'previous' | 'select' | 'interrupt' | 'searchSessions'
  | 'searchFiles' | 'quickOpen' | 'composer' | 'sidebar' | 'panel' | 'rightPanel' | 'zoomIn' | 'zoomOut'
  | 'zoomReset' | 'wrap' | 'nextChange' | 'previousChange' | 'find' | 'findNext' | 'findPrevious' | 'help';

export type Section = 'App' | 'Session' | 'View' | 'Help';

// Shortcut is one key binding. menu puts it in the desktop app's menus, which
// then own every key of it, native has the app handle it, and terminal keeps
// it working while a terminal has focus.
interface Shortcut {
  id: ShortcutId;
  section: Section;
  title: string;
  keys: string[];
  menu?: boolean;
  native?: boolean;
  terminal?: boolean;
}

// Bindings also supply the desktop app's menus and the keyboard reference.
export const shortcuts: Shortcut[] = [
  {id: 'settings', section: 'App', title: 'Settings…', keys: ['⌘,'], menu: true, terminal: true},
  {id: 'new', section: 'Session', title: 'New Session', keys: ['⌘N'], menu: true},
  {id: 'next', section: 'Session', title: 'Next Session', keys: ['⌘⌥↓', '⌃Tab'], menu: true},
  {id: 'previous', section: 'Session', title: 'Previous Session', keys: ['⌘⌥↑', '⌃⇧Tab'], menu: true},
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
  {id: 'nextChange', section: 'View', title: 'Next Change', keys: ['⌥F5']},
  {id: 'previousChange', section: 'View', title: 'Previous Change', keys: ['⌥⇧F5']},
  {id: 'find', section: 'View', title: 'Find', keys: ['⌘F']},
  {id: 'findNext', section: 'View', title: 'Next Match', keys: ['⌘G', 'F3']},
  {id: 'findPrevious', section: 'View', title: 'Previous Match', keys: ['⌘⇧G', '⇧F3']},
  {id: 'help', section: 'Help', title: 'Keyboard Shortcuts', keys: ['⌘/'], menu: true},
];

// Translate the notation used by the reference into keyboard event fields.
const arrows: Record<string, string> = {'↓': 'ArrowDown', '↑': 'ArrowUp'};

export function binding(text: string) {
  const key = text.replace(/[⌘⌥⌃⇧]/g, '');
  return {key: arrows[key] ?? key.toLowerCase(),
    metaKey: text.includes('⌘'), altKey: text.includes('⌥'), ctrlKey: text.includes('⌃'), shiftKey: text.includes('⇧') || key === '+'};
}

// A keystroke, from a DOM or React keyboard event.
interface Keystroke {
  key: string;
  code?: string;
  metaKey?: boolean;
  altKey?: boolean;
  ctrlKey?: boolean;
  shiftKey?: boolean;
}

export function matches(event: Keystroke, id: ShortcutId) {
  return shortcutOf(id).keys.some(text => {
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

// firstKeys is the notation of a shortcut's first binding.
export const firstKeys = (id: ShortcutId) => shortcutOf(id).keys[0];

export interface ShortcutActions {
  focusComposer: () => void;
  showHelp: () => void;
  showSettings: () => void;
  openQuickOpen: () => void;
  openSessionSearch: () => void;
}

const regionSelectors: Record<Region, string> = {sidebar: '.sidebar', panel: '.terminal-panel', rightPanel: '.right-panel'};

// shortcutAction is what shortcut id does for a keystroke in target, or
// undefined when it does nothing there. editing says the keystroke is in a text
// field, which keeps its own keys.
function shortcutAction(id: ShortcutId, target: Element, editing: boolean, key: string, store: Store, actions: ShortcutActions): (() => void) | undefined {
  const state = store.get();
  switch (id) {
    case 'wrap':
      return editing ? undefined : () => store.update(toggleWrap);
    case 'searchSessions':
      return actions.openSessionSearch;
    case 'searchFiles':
      if (!state.session || state.unsent) return undefined;
      return () => {
        store.update(s => ({...s, workbench: {...s.workbench, rightPanel: true, rightTab: 'search'}}));
        if (state.workbench.rightPanel && state.workbench.rightTab === 'search') document.querySelector<HTMLElement>('[aria-label="Search files contents"]')?.focus();
      };
    case 'quickOpen':
      return state.session && !state.unsent ? actions.openQuickOpen : undefined;
    case 'new':
      return () => store.update(s => openUnsent(s));
    case 'composer':
      return actions.focusComposer;
    case 'select': {
      const session = state.workspaces.flatMap(w => w.sessions.filter(s => !s.ended))[Number(key) - 1];
      return session && (() => chooseSession(store, session.id));
    }
    case 'next':
    case 'previous': {
      const rows = [...document.querySelectorAll<HTMLElement>('.sidebar [role="treeitem"]')];
      const index = rows.findIndex(row => row.dataset.session === state.selected);
      const previous = id === 'previous';
      const session = rows[index < 0 ? (previous ? rows.length - 1 : 0) : (index + (previous ? -1 : 1) + rows.length) % rows.length]?.dataset.session;
      return session ? () => chooseSession(store, session) : undefined;
    }
    case 'sidebar':
    case 'panel':
    case 'rightPanel':
      if (editing) return undefined;
      return () => {
        if (state.workbench[id] && target.closest(regionSelectors[id])) actions.focusComposer();
        store.update(s => toggle(s, id));
      };
    case 'settings':
      return actions.showSettings;
    case 'help':
      return actions.showHelp;
    default:
      return undefined;
  }
}

const editingTarget = (target: Element) => !!target.closest('input, textarea, [contenteditable="true"]');

// shortcut runs the shortcut that a key press in the page names. In the
// desktop app the menus run the shortcuts they own.
export function shortcut(e: KeyboardEvent, store: Store, actions: ShortcutActions) {
  if (e.defaultPrevented || e.repeat || e.isComposing || !(e.target instanceof Element)) return;
  if (e.target.closest('[role=menu], [role=dialog], [role=alertdialog]')) return;
  if (e.target.closest('.terminal-panel') && !shortcuts.some(item => item.terminal && matches(e, item.id))) return;
  if (store.get().confirmation) return;
  const item = shortcuts.find(s => matches(e, s.id));
  if (item?.menu && hypercube()) return;
  const action = item && shortcutAction(item.id, e.target, editingTarget(e.target), e.key, store, actions);
  if (!action) return;
  e.preventDefault();
  e.stopPropagation();
  action();
}

// nativeShortcut runs the shortcut that the app's menu names, for the element
// that has focus.
export function nativeShortcut(id: string, store: Store, actions: ShortcutActions) {
  const item = shortcuts.find(s => s.id === id);
  if (item) shortcutAction(item.id, document.activeElement ?? document.body, false, '', store, actions)?.();
}
