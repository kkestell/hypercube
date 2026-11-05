import {afterEach, expect, test} from 'bun:test';
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
