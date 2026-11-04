import {expect, test} from 'bun:test';
import {startAppearance} from './appearance.js';

test('the page follows the system appearance as it changes', () => {
  const original = globalThis.matchMedia;
  let change;
  const media = {matches: false, addEventListener: (type, listener) => { change = listener; }};
  globalThis.matchMedia = () => media;
  try {
    startAppearance();
    expect(document.documentElement.classList.contains('dark')).toBe(false);
    media.matches = true;
    change();
    expect(document.documentElement.classList.contains('dark')).toBe(true);
    media.matches = false;
    change();
    expect(document.documentElement.classList.contains('dark')).toBe(false);
  } finally {
    globalThis.matchMedia = original;
  }
});
