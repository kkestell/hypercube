import {chooseSession} from '../sessions/session-actions.js';
import {openUnsent, runningFirst} from '../sessions/selection.js';
import {toggle} from './workbench.js';
import {windowHandler} from './window.js';

// Bindings also supply the native menus and the keyboard reference.
export const shortcuts = [
  {id: 'new', section: 'Session', title: 'New Session', keys: ['⌘N'], menu: true},
  {id: 'next', section: 'Session', title: 'Next Session', keys: ['⌃Tab', '⌘⌥↓'], menu: true, menuKey: 1},
  {id: 'previous', section: 'Session', title: 'Previous Session', keys: ['⌃⇧Tab', '⌘⌥↑'], menu: true, menuKey: 1},
  {id: 'select', section: 'Session', title: 'Select Session', keys: ['⌘1–9']},
  {id: 'interrupt', section: 'Session', title: 'Interrupt Agent', keys: ['Escape']},
  {id: 'quickOpen', section: 'Session', title: 'Quick Open', keys: ['⌘P']},
  {id: 'composer', section: 'Session', title: 'Focus Composer', keys: ['⌘L']},
  {id: 'sidebar', section: 'View', title: 'Toggle Sidebar', keys: ['⌘B'], menu: true},
  {id: 'panel', section: 'View', title: 'Toggle Terminal', keys: ['⌘J'], menu: true},
  {id: 'rightPanel', section: 'View', title: 'Toggle Right Panel', keys: ['⌘⌥B'], menu: true},
  {id: 'zoomIn', section: 'View', title: 'Zoom In', keys: ['⌘=', '⌘+'], menu: true, native: true, menuAlternates: true},
  {id: 'zoomOut', section: 'View', title: 'Zoom Out', keys: ['⌘-'], menu: true, native: true},
  {id: 'zoomReset', section: 'View', title: 'Actual Size', keys: ['⌘0'], menu: true, native: true},
  {id: 'find', section: 'View', title: 'Find in Transcript', keys: ['⌘F']},
  {id: 'findNext', section: 'View', title: 'Next Match', keys: ['⌘G', 'F3']},
  {id: 'findPrevious', section: 'View', title: 'Previous Match', keys: ['⌘⇧G', '⇧F3']},
  {id: 'help', section: 'Help', title: 'Keyboard Shortcuts', keys: ['⌘/'], menu: true},
];

// Translate the notation used by the reference into keyboard event fields.
export function binding(text) {
  const key = text.replace(/[⌘⌥⌃⇧]/g, '');
  return {key: ({'↓': 'ArrowDown', '↑': 'ArrowUp'})[key] ?? key.toLowerCase(),
    metaKey: text.includes('⌘'), altKey: text.includes('⌥'), ctrlKey: text.includes('⌃'), shiftKey: text.includes('⇧') || key === '+'};
}
export function matches(event, id) {
  const item = shortcuts.find(s => s.id === id);
  return item.keys.some(text => {
    const b = binding(text);
    const key = event.altKey && event.code === 'KeyB' ? 'b' : event.key.toLowerCase();
    const range = b.key.match(/^(.)–(.)$/);
    const keyMatches = range ? key.length === 1 && key >= range[1] && key <= range[2] : key === b.key.toLowerCase();
    return keyMatches && !!event.metaKey === b.metaKey && !!event.altKey === b.altKey && !!event.ctrlKey === b.ctrlKey && !!event.shiftKey === b.shiftKey;
  });
}

export function shortcut(e, store, focusComposer, showHelp, id, openQuickOpen) {
  if (!id && (e.defaultPrevented || e.repeat || e.isComposing || e.nativeEvent?.isComposing || (e.target.closest('[role=menu], [role=dialog], [role=alertdialog]') || (e.target.closest('.terminal-panel') && !matches(e, 'quickOpen'))))) return;
  const state = store.get();
  if (!id && state.confirmation) return;
  const is = name => id ? id === name : matches(e, name);
  const field = e.target.closest('input, textarea, [contenteditable="true"]');
  let action;
  if (is('quickOpen') && state.session && !state.unsent) action = openQuickOpen;
  else if (is('new')) action = () => store.update(s => openUnsent(s));
  else if (is('composer')) action = focusComposer;
  else if (is('select')) {
    const session = state.workspaces.flatMap(w => runningFirst(w.sessions))[Number(e.key) - 1];
    if (session) action = () => chooseSession(store, session.id);
  } else if (is('next') || is('previous')) {
    const rows = [...document.querySelectorAll('.sidebar [role="treeitem"]')];
    const index = rows.findIndex(row => row.dataset.session === state.selected);
    const previous = is('previous');
    const row = rows[(index < 0 ? (previous ? rows.length - 1 : 0) : (index + (previous ? -1 : 1) + rows.length) % rows.length)];
    if (row) action = () => chooseSession(store, row.dataset.session);
  } else if ((id || !field) && ['sidebar', 'panel', 'rightPanel'].some(is)) {
    const region = ['sidebar', 'panel', 'rightPanel'].find(is);
    const selector = {sidebar: '.sidebar', panel: '.terminal-panel', rightPanel: '.right-panel'}[region];
    action = () => {
      if (state.workbench[region] && e.target.closest(selector)) focusComposer();
      store.update(s => toggle(s, region));
    };
  } else if (is('help')) action = showHelp;
  else if (windowHandler() && ['zoomIn', 'zoomOut', 'zoomReset'].some(is)) {
    const zoom = ['zoomIn', 'zoomOut', 'zoomReset'].find(is);
    action = () => windowHandler().postMessage({type: 'page-zoom', action: zoom});
  }
  if (action) { e.preventDefault(); e.stopPropagation(); action(); }
}
