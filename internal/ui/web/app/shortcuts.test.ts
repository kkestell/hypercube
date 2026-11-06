import {expect, test} from 'bun:test';
import {testState} from '../../test-state.ts';
import {createStore} from '../store.ts';
import {nativeShortcut, shortcut, type ShortcutActions} from './shortcuts.ts';

const none = () => {};
const actions: ShortcutActions = {focusComposer: none, showHelp: none, showSettings: none, openQuickOpen: none, openSessionSearch: none};
const keyDown = (key: string, code: string) => ({key, code, metaKey: true, target: document.body, defaultPrevented: false, repeat: false,
  isComposing: false, preventDefault: none, stopPropagation: none}) as unknown as KeyboardEvent;

test('the desktop app\'s menus run the shortcuts they own, once', () => {
  const store = createStore(testState());
  const sidebar = () => store.get().workbench.sidebar;
  const before = sidebar();

  shortcut(keyDown('b', 'KeyB'), store, actions);
  expect(sidebar()).toBe(!before);

  globalThis.hypercube = {postMessage: none, onEvent: none};
  try {
    shortcut(keyDown('b', 'KeyB'), store, actions);
    expect(sidebar()).toBe(!before);
    nativeShortcut('sidebar', store, actions);
    expect(sidebar()).toBe(before);
  } finally {
    delete globalThis.hypercube;
  }
});
