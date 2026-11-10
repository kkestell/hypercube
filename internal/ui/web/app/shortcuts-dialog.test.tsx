import {afterEach, expect, test} from 'bun:test';
import {mount, testBridge, unmountAll} from '../../test-render.tsx';
import {testState} from '../../test-state.ts';
import {createStore} from '../store.ts';
import {ShortcutsDialog} from './shortcuts-dialog.tsx';

afterEach(() => {
  unmountAll();
  delete globalThis.hypercube;
});

// searchFiles is the Search Files row's keycaps and the text between them.
function searchFiles() {
  mount(createStore(testState({dialog: 'shortcuts'})), <ShortcutsDialog />);
  const row = [...document.querySelectorAll('dl > div')].find(item => item.querySelector('dt')?.textContent === 'Search Files');
  const group = row?.querySelector('[data-slot=kbd-group]');
  return {keys: [...group?.querySelectorAll('kbd') ?? []].map(key => key.textContent), text: group?.textContent};
}

test('the shortcut reference shows macOS glyphs', () => {
  expect(searchFiles()).toEqual({keys: ['⌘', '⇧', 'F'], text: '⌘⇧F'});
});

test('the shortcut reference shows Linux key names', () => {
  globalThis.hypercube = testBridge({platform: 'linux'});
  expect(searchFiles()).toEqual({keys: ['Ctrl', 'Shift', 'F'], text: 'Ctrl+Shift+F'});
});
