const {expect, test} = require('bun:test');
const {accelerator} = require('./accelerator.js');

const binding = (key, ...modifiers) => ({key, metaKey: modifiers.includes('meta'), altKey: modifiers.includes('alt'),
  ctrlKey: modifiers.includes('ctrl'), shiftKey: modifiers.includes('shift')});

test.each([
  [binding(',', 'meta'), 'Command+,'],
  [binding('n', 'meta'), 'Command+N'],
  [binding('b', 'meta', 'alt'), 'Command+Alt+B'],
  [binding('ArrowDown', 'meta', 'alt'), 'Command+Alt+Down'],
  [binding('tab', 'ctrl', 'shift'), 'Control+Shift+Tab'],
  [binding('pagedown', 'ctrl'), 'Control+PageDown'],
  [binding('f7', 'shift'), 'Shift+F7'],
  [binding('f3'), 'F3'],
  [binding('=', 'meta'), 'Command+='],
  [binding('+', 'meta', 'shift'), 'Command+Plus'],
])('%o is the accelerator %p', (b, expected) => {
  expect(accelerator(b)).toBe(expected);
});
