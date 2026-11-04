import {chooseSession} from '../sessions/session-actions.js';
import {openUnsent, runningFirst} from '../sessions/selection.js';
import {toggle} from './workbench.js';

// The tree renders only rows exposed by its workspace and parent sections.
export function shortcut(e, store, focusComposer, openQuickOpen) {
  if (e.defaultPrevented || e.repeat || (e.isComposing || e.nativeEvent?.isComposing) || e.target.closest('[role=menu], [role=dialog], [role=alertdialog]')) return;
  if (e.target.closest('.terminal-panel') && !(e.metaKey && !e.ctrlKey && !e.altKey && !e.shiftKey && e.key.toLowerCase() === 'p')) return;
  const state = store.get();
  if (state.confirmation) return;
  const key = e.altKey && e.code === 'KeyB' ? 'b' : e.key.toLowerCase();
  const command = e.metaKey && !e.ctrlKey && !e.shiftKey;
  const field = e.target.closest('input, textarea, [contenteditable="true"]');
  let action;
  if (command && !e.altKey && key === 'p' && state.session && !state.unsent) action = openQuickOpen;
  else if (command && !e.altKey && key === 'n') action = () => store.update(s => openUnsent(s));
  else if (command && !e.altKey && key === 'l') action = focusComposer;
  else if (command && !e.altKey && /^[1-9]$/.test(key)) {
    const session = state.workspaces.flatMap(w => runningFirst(w.sessions))[Number(key) - 1];
    if (session) action = () => chooseSession(store, session.id);
  } else if ((!e.metaKey && e.ctrlKey && !e.altKey && key === 'tab') ||
             (command && e.altKey && (key === 'arrowdown' || key === 'arrowup'))) {
    const rows = [...document.querySelectorAll('.sidebar [role="treeitem"]')];
    const index = rows.findIndex(row => row.dataset.session === state.selected);
    const previous = e.shiftKey || key === 'arrowup';
    const row = rows[(index < 0 ? (previous ? rows.length - 1 : 0) : (index + (previous ? -1 : 1) + rows.length) % rows.length)];
    if (row) action = () => chooseSession(store, row.dataset.session);
  } else if (command && !field && ((key === 'b' && !e.altKey) || (key === 'j' && !e.altKey) || (key === 'b' && e.altKey))) {
    const region = key === 'j' ? 'panel' : e.altKey ? 'rightPanel' : 'sidebar';
    const selector = {sidebar: '.sidebar', panel: '.terminal-panel', rightPanel: '.right-panel'}[region];
    action = () => {
      if (state.workbench[region] && e.target.closest(selector)) focusComposer();
      store.update(s => toggle(s, region));
    };
  }
  if (action) { e.preventDefault(); e.stopPropagation(); action(); }
}
