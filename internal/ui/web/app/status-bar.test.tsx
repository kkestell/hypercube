import {afterEach, expect, test} from 'bun:test';
import {mount, unmountAll} from '../../test-render.tsx';
import {testState} from '../../test-state.ts';
import {createStore} from '../store.ts';
import {StatusBar, timeLeft} from './status-bar.tsx';
import type {Limit} from './types.ts';

afterEach(unmountAll);

const now = Math.floor(Date.now() / 1000);
const hour = 3600;
const limit = (overrides: Partial<Limit>): Limit => ({harness: 'claude', minutes: 300, used: 0, at: 0, ...overrides});
const render = (limits: Partial<Limit>[]) => mount(createStore(testState({limits: limits.map(limit)})), <StatusBar />);
const harnesses = (root: HTMLElement) => [...root.querySelectorAll('.status-bar-harness')].map(h => h.firstChild?.textContent);
const windows = (root: HTMLElement) => [...root.querySelectorAll<HTMLElement>('.status-bar-window')];
const ahead = (window: HTMLElement) => window.classList.contains('status-bar-window-ahead');

test('each harness shows its windows with the percent used and the time left', () => {
  const root = render([
    {harness: 'claude', minutes: 300, used: 42, resets: now + 2 * hour},
    {harness: 'claude', minutes: 10080, used: 18, resets: now + 52 * hour},
    {harness: 'codex', minutes: 10080, used: 48, resets: now + 30},
  ]);
  expect(harnesses(root)).toEqual(['Claude', 'Codex']);
  expect(windows(root).map(w => w.textContent)).toEqual(['5h42%2h 0m', '7d18%2d 4h', '7d48%1m']);
  expect(root.querySelector('.status-bar-window-reset')).toBeNull();
});

test('usage bars mark elapsed time, and an icon marks usage ahead of pace', () => {
  const root = render([
    // 3 of 5 hours elapsed: 60% is exact pace.
    {harness: 'claude', minutes: 300, used: 70, resets: now + 2 * hour},
    {harness: 'claude', minutes: 600, used: 50, resets: now + 4 * hour},
  ]);
  const [over, under] = windows(root);
  expect(over.querySelectorAll('[role="progressbar"]')).toHaveLength(1);
  expect(over.querySelector('[aria-label="5h elapsed: 60%"]')).not.toBeNull();
  expect(over.querySelector<HTMLElement>('.status-bar-meter-pace')?.style.left).toContain('60');
  expect(ahead(over)).toBe(true);
  expect(ahead(under)).toBe(false);
  expect(over.title).toBe('5h limit: 70% used, 60% of the window elapsed, resets in 2h 0m');
});

test('a window past its reset keeps its percent, dimmed, without pace or time left', () => {
  const [passed] = windows(render([{harness: 'claude', minutes: 300, used: 42, resets: now - 60}]));
  expect(passed.classList.contains('status-bar-window-reset')).toBe(true);
  expect(passed.querySelector('.status-bar-meter-pace, .status-bar-left')).toBeNull();
  expect(passed.title).toBe('5h limit: 42% used, reset');
});

test('a window with an unknown reset shows no pace or time left', () => {
  const [window] = windows(render([{harness: 'codex', minutes: 10080, used: 48, resets: 0}]));
  expect(window.querySelector('.status-bar-meter-pace, .status-bar-left')).toBeNull();
  expect(window.title).toBe('7d limit: 48% used');
});

test.each([
  [52 * hour * 1000, '2d 4h'],
  [(3 * hour + 11 * 60 + 1) * 1000, '3h 12m'],
  [1000, '1m'],
])('timeLeft(%i) is %s', (ms, text) => {
  expect(timeLeft(ms)).toBe(text);
});

test('without limits the footer shows only the layout toggles', () => {
  const root = render([]);
  expect(root.querySelector('.status-bar-harness')).toBeNull();
  expect(root.querySelectorAll('.status-bar .layout-toggles button')).toHaveLength(3);
});
