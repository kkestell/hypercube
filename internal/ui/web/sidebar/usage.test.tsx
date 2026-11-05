import {afterEach, expect, test} from 'bun:test';
import {act} from 'react';
import {mount, unmountAll} from '../../test-render.tsx';
import {testState} from '../../test-state.ts';
import {createStore} from '../store.ts';
import {Usage} from './usage.tsx';

afterEach(unmountAll);

test('both reported windows are available for the hover and focus switch', () => {
  const root = mount(createStore(testState({limits: [
    {harness: 'codex', minutes: 10080, used: 25, at: 1},
    {harness: 'codex', minutes: 300, used: 10, at: 1},
  ]})), <Usage />);
  const bars = [...root.querySelectorAll('[role=progressbar]')];
  expect(bars.map(bar => bar.getAttribute('aria-label'))).toEqual(['codex 5h usage', 'codex 7d usage']);
  expect(bars.map(bar => bar.getAttribute('aria-valuenow'))).toEqual(['10', '25']);
});

test('a weekly-only report shows its actual window without inventing a five-hour reading', () => {
  const root = mount(createStore(testState({limits: [
    {harness: 'codex', minutes: 10080, used: 48, at: 1},
  ]})), <Usage />);
  const bars = root.querySelectorAll('[role=progressbar]');
  expect(bars).toHaveLength(1);
  expect(bars[0].getAttribute('aria-label')).toBe('codex 7d usage');
});

test('Claude keeps its five-hour reading after reset until a new report arrives', () => {
  const now = Math.floor(Date.now() / 1000);
  const store = createStore(testState({limits: [
    {harness: 'claude', minutes: 300, used: 4, resets: now - 3600, at: 1},
    {harness: 'claude', minutes: 10080, used: 44, resets: now + 86400, at: 1},
  ]}));
  const root = mount(store, <Usage />);
  const bars = [...root.querySelectorAll('[role=progressbar]')];
  expect(bars.map(bar => bar.getAttribute('aria-label'))).toEqual(['claude 5h usage', 'claude 7d usage']);
  expect(bars[0].getAttribute('aria-valuenow')).toBe('4');
  expect(bars[0].getAttribute('aria-valuetext')).toContain('last reported');
  expect(bars[0].getAttribute('aria-valuetext')).toContain('Waiting for a new reading from claude');
  expect(root.textContent).toContain('Waiting');
  act(() => store.update(state => ({...state, limits: state.limits.map(limit => limit.minutes === 300
    ? {...limit, used: 1, resets: now + 18000, at: 2} : limit)})));
  expect(bars[0].getAttribute('aria-valuenow')).toBe('1');
  expect(bars[0].getAttribute('aria-valuetext')).toBeNull();
  expect(root.textContent).not.toContain('Waiting');
});
