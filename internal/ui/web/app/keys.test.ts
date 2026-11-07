import {afterEach, expect, test} from 'bun:test';
import {testBridge} from '../../test-render.tsx';
import {binding, keysLabel} from './keys.ts';

afterEach(() => {
  delete globalThis.hypercube;
});

const fields = (key: string, ...on: ('metaKey' | 'altKey' | 'ctrlKey' | 'shiftKey')[]) =>
  ({key, metaKey: on.includes('metaKey'), altKey: on.includes('altKey'), ctrlKey: on.includes('ctrlKey'), shiftKey: on.includes('shiftKey')});

const cases: [string, ReturnType<typeof fields>, ReturnType<typeof fields>, string][] = [
  ['⌘,', fields(',', 'metaKey'), fields(',', 'ctrlKey'), 'Ctrl+,'],
  ['⌘⇧F', fields('f', 'metaKey', 'shiftKey'), fields('f', 'ctrlKey', 'shiftKey'), 'Ctrl+Shift+F'],
  ['⌘⌥B', fields('b', 'metaKey', 'altKey'), fields('b', 'ctrlKey', 'altKey'), 'Alt+Ctrl+B'],
  ['⌘⌥↓', fields('ArrowDown', 'metaKey', 'altKey'), fields('ArrowDown', 'ctrlKey', 'altKey'), 'Alt+Ctrl+Down'],
  ['⌃⇧Tab', fields('tab', 'ctrlKey', 'shiftKey'), fields('tab', 'ctrlKey', 'shiftKey'), 'Ctrl+Shift+Tab'],
  ['⌥Z', fields('z', 'altKey'), fields('z', 'altKey'), 'Alt+Z'],
  ['⌘+', fields('+', 'metaKey', 'shiftKey'), fields('+', 'ctrlKey', 'shiftKey'), 'Ctrl++'],
  ['⌘1–9', fields('1–9', 'metaKey'), fields('1–9', 'ctrlKey'), 'Ctrl+1–9'],
  ['F3', fields('f3'), fields('f3'), 'F3'],
  ['Escape', fields('escape'), fields('escape'), 'Escape'],
];

test('macOS reads ⌘ as Command and labels with glyphs', () => {
  for (const [text, fieldsOf] of cases) {
    expect(binding(text)).toEqual(fieldsOf);
    expect(keysLabel(text)).toBe(text);
  }
});

test('Linux reads ⌘ as Ctrl, never sets metaKey, and labels with names', () => {
  globalThis.hypercube = testBridge({platform: 'linux'});
  for (const [text, , fieldsOf, label] of cases) {
    expect(binding(text)).toEqual(fieldsOf);
    expect(keysLabel(text)).toBe(label);
  }
});
