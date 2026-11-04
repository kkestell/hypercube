import {afterEach, expect, test} from 'bun:test';
import {mount, unmountAll} from '../../test-render.jsx';
import {createStore} from '../store.js';
import {UsageLimits} from './usage-limits.jsx';

afterEach(unmountAll);

const now = Math.floor(Date.now() / 1000);
const render = limits => mount(createStore({limits}), <UsageLimits />);
const rows = root => [...root.querySelectorAll('.usage-limits-row')].map(row => row.textContent);
const windows = root => [...root.querySelectorAll('.usage-limits-window')];

test('one row per harness, windows by length, with reset times', () => {
  const root = render([
    {harness: 'claude', minutes: 300, used: 42, resets: now + 3600},
    {harness: 'claude', minutes: 10080, used: 18, resets: now + 86400},
    {harness: 'codex', minutes: 10080, used: 48, resets: now + 7200},
  ]);
  expect(rows(root)).toEqual(['Claude5h 42% · 7d 18%', 'Codex7d 48%']);
  expect(windows(root).map(w => w.title.startsWith('Resets '))).toEqual([true, true, true]);
  expect(root.querySelector('.usage-limits-window-reset')).toBeNull();
});

test('a window past its reset keeps its percent and is marked reset', () => {
  const root = render([
    {harness: 'claude', minutes: 300, used: 42, resets: now - 60},
    {harness: 'claude', minutes: 10080, used: 18, resets: now + 86400},
  ]);
  expect(rows(root)).toEqual(['Claude5h 42% · 7d 18%']);
  const [passed, open] = windows(root);
  expect(passed.classList.contains('usage-limits-window-reset')).toBe(true);
  expect(passed.title.startsWith('Reset ')).toBe(true);
  expect(open.classList.contains('usage-limits-window-reset')).toBe(false);
});

test('a window with an unknown reset has no reset title', () => {
  const root = render([{harness: 'codex', minutes: 10080, used: 48, resets: 0}]);
  expect(rows(root)).toEqual(['Codex7d 48%']);
  expect(windows(root)[0].hasAttribute('title')).toBe(false);
});

test('nothing shows without limits', () => {
  const root = render([]);
  expect(root.querySelector('.usage-limits')).toBeNull();
});
