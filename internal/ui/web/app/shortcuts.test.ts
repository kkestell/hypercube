import {afterEach, expect, test} from 'bun:test';
import {testBridge} from '../../test-render.tsx';
import {testState} from '../../test-state.ts';
import {createStore} from '../store.ts';
import {binding} from './keys.ts';
import {keysOf, matches, nativeShortcut, shortcut, shortcuts, terminalShortcut, type ShortcutActions} from './shortcuts.ts';

afterEach(() => {
  delete globalThis.hypercube;
});

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

  globalThis.hypercube = testBridge();
  try {
    shortcut(keyDown('b', 'KeyB'), store, actions);
    expect(sidebar()).toBe(!before);
    nativeShortcut('sidebar', store, actions);
    expect(sidebar()).toBe(before);
  } finally {
    delete globalThis.hypercube;
  }
});

const useLinux = () => { globalThis.hypercube = testBridge({platform: 'linux'}); };

const stroke = (b: ReturnType<typeof binding>) => [b.key, b.metaKey, b.altKey, b.ctrlKey, b.shiftKey].join();
const strokeOf = (text: string) => stroke(binding(text));

// keystrokes are the keystrokes of every binding in the table, with 1–9 expanded.
const keystrokes = () => shortcuts.flatMap(item => keysOf(item).flatMap(text => {
  const b = binding(text);
  const range = b.key.match(/^(.)–(.)$/);
  const keys = range ? Array.from({length: Number(range[2]) - Number(range[1]) + 1}, (_, i) => String(Number(range[1]) + i)) : [b.key];
  return keys.map(key => ({id: item.id, stroke: stroke({...b, key})}));
}));

// Edit menu keys, which belong to copy, paste, cut, select all, and undo. The
// terminal's Ctrl+Shift+C and Ctrl+Shift+V are reserved on Linux.
const editKeys = () => ['⌘C', '⌘V', '⌘X', '⌘A', '⌘Z', '⌘⇧Z'].map(strokeOf);
const terminalKeys = () => ['⌃⇧C', '⌃⇧V'].map(strokeOf);

test('no two bindings share a keystroke, and none takes an editing key', () => {
  for (const platform of ['darwin', 'linux']) {
    globalThis.hypercube = testBridge({platform});
    const strokes = keystrokes();
    expect(new Set(strokes.map(k => k.stroke)).size).toBe(strokes.length);
    for (const {id, stroke} of strokes) {
      expect([id, editKeys().includes(stroke)]).toEqual([id, false]);
      if (platform === 'linux') expect([id, terminalKeys().includes(stroke)]).toEqual([id, false]);
    }
  }
});

const press = (key: string, modifiers: {ctrlKey?: boolean; altKey?: boolean; shiftKey?: boolean; metaKey?: boolean}, target: Element = document.body) =>
  ({key, code: '', target, defaultPrevented: false, repeat: false, isComposing: false, preventDefault: none, stopPropagation: none, ...modifiers}) as unknown as KeyboardEvent;

test('on Linux the bindings use Ctrl and avoid GNOME\'s', () => {
  useLinux();
  const store = createStore(testState());
  let searched = 0;
  shortcut(press('k', {ctrlKey: true}), store, {...actions, openSessionSearch: () => { searched++; }});
  expect(searched).toBe(1);

  expect(matches(press('ArrowDown', {ctrlKey: true, altKey: true}), 'next')).toBe(false);
  expect(matches(press('F5', {altKey: true}), 'nextChange')).toBe(false);
  expect(matches(press('PageDown', {ctrlKey: true}), 'next')).toBe(true);
  expect(matches(press('Tab', {ctrlKey: true}), 'next')).toBe(true);
  expect(matches(press('F7', {}), 'nextChange')).toBe(true);
  expect(matches(press('F7', {shiftKey: true}), 'previousChange')).toBe(true);
  expect(matches(press('b', {metaKey: true}), 'sidebar')).toBe(false);
});

test('the terminal panel keeps every key except the shortcuts marked terminal', () => {
  useLinux();
  expect(terminalShortcut(press('b', {ctrlKey: true}))).toBe(false);
  expect(terminalShortcut(press('p', {ctrlKey: true}))).toBe(true);
  expect(terminalShortcut(press(',', {ctrlKey: true}))).toBe(true);
  expect(terminalShortcut(press('F', {ctrlKey: true, shiftKey: true}))).toBe(true);

  const panel = document.createElement('div');
  panel.className = 'terminal-panel';
  document.body.append(panel);
  let searched = 0;
  try {
    shortcut(press('k', {ctrlKey: true}, panel), createStore(testState()), {...actions, openSessionSearch: () => { searched++; }});
  } finally {
    panel.remove();
  }
  expect(searched).toBe(0);
});
