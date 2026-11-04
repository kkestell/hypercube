import {expect, test} from 'bun:test';
import {startAppearance} from './appearance.ts';

test('the page follows the system appearance as it changes', () => {
  const original = globalThis.matchMedia;
  const listeners: (() => void)[] = [];
  const change = () => listeners.forEach(listener => listener());
  const media = {matches: false, addEventListener: (_type: string, listener: () => void) => { listeners.push(listener); }};
  Object.defineProperty(globalThis, 'matchMedia', {value: () => media, configurable: true, writable: true});
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
    Object.defineProperty(globalThis, 'matchMedia', {value: original, configurable: true, writable: true});
  }
});
