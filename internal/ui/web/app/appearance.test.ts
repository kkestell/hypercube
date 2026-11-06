import {expect, test} from 'bun:test';
import {testConfig, testState} from '../../test-state.ts';
import {createStore} from '../store.ts';
import {startAppearance} from './appearance.ts';

test('the page follows the Mac until a theme overrides it', () => {
  const original = globalThis.matchMedia;
  const listeners: (() => void)[] = [];
  const change = (matches: boolean) => { media.matches = matches; listeners.forEach(listener => listener()); };
  const media = {matches: false, addEventListener: (_type: string, listener: () => void) => { listeners.push(listener); }};
  Object.defineProperty(globalThis, 'matchMedia', {value: () => media, configurable: true, writable: true});
  const dark = () => document.documentElement.classList.contains('dark');
  try {
    const store = createStore(testState());
    startAppearance(store);
    expect(dark()).toBe(false);
    change(true);
    expect(dark()).toBe(true);
    store.update(state => ({...state, config: testConfig({theme: 'light'})}));
    expect(dark()).toBe(false);
    store.update(state => ({...state, config: testConfig({theme: 'dark'})}));
    change(false);
    expect(dark()).toBe(true);
    store.update(state => ({...state, config: testConfig({theme: 'system'})}));
    expect(dark()).toBe(false);
  } finally {
    Object.defineProperty(globalThis, 'matchMedia', {value: original, configurable: true, writable: true});
  }
});
