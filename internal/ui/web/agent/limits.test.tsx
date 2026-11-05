import {afterEach, expect, jest, test} from 'bun:test';
import {act} from 'react';
import {mount, unmountAll} from '../../test-render.tsx';
import {testState} from '../../test-state.ts';
import {createStore} from '../store.ts';
import {Limits} from './limits.tsx';

afterEach(() => {
  unmountAll();
  jest.useRealTimers();
});

test("both of the session's harness's windows are available for the hover and focus switch", () => {
  const root = mount(createStore(testState({limits: [
    {harness: 'codex', minutes: 10080, used: 25, at: 1},
    {harness: 'claude', minutes: 300, used: 70, at: 1},
    {harness: 'codex', minutes: 300, used: 10, at: 1},
  ]})), <Limits harness="codex" />);
  const bars = [...root.querySelectorAll('[role=progressbar]')];
  expect(bars.map(bar => bar.getAttribute('aria-label'))).toEqual(['codex 5h usage', 'codex 7d usage']);
  expect(bars.map(bar => bar.getAttribute('aria-valuenow'))).toEqual(['10', '25']);
});

test('a weekly-only report shows its actual window without inventing a five-hour reading', () => {
  const root = mount(createStore(testState({limits: [
    {harness: 'codex', minutes: 10080, used: 48, at: 1},
  ]})), <Limits harness="codex" />);
  const bars = root.querySelectorAll('[role=progressbar]');
  expect(bars).toHaveLength(1);
  expect(bars[0].getAttribute('aria-label')).toBe('codex 7d usage');
});

test('a harness that reports no limits shows nothing', () => {
  const root = mount(createStore(testState({limits: [{harness: 'claude', minutes: 300, used: 4, at: 1}]})), <Limits harness="pi" />);
  expect(root.querySelector('.limits')).toBeNull();
});

test('Claude keeps its five-hour reading after reset until a new report arrives', () => {
  const now = Math.floor(Date.now() / 1000);
  const store = createStore(testState({limits: [
    {harness: 'claude', minutes: 300, used: 4, resets: now - 3600, at: 1},
    {harness: 'claude', minutes: 10080, used: 44, resets: now + 86400, at: 1},
  ]}));
  const root = mount(store, <Limits harness="claude" />);
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

test('an idle meter counts down and waits at reset without a store update', () => {
  const now = 1800000000000;
  jest.useFakeTimers({now});
  const root = mount(createStore(testState({limits: [
    {harness: 'claude', minutes: 300, used: 42, resets: now / 1000 + 90, at: now},
  ]})), <Limits harness="claude" />);
  const bar = root.querySelector('[role=progressbar]')!;
  expect(root.textContent).toContain('2m');
  act(() => { jest.advanceTimersByTime(30000); });
  expect(root.textContent).toContain('1m');
  act(() => { jest.advanceTimersByTime(60000); });
  expect(root.textContent).toContain('Waiting');
  expect(bar.getAttribute('aria-valuenow')).toBe('42');
  expect(bar.getAttribute('aria-valuetext')).toContain('this window reset');
  expect(jest.getTimerCount()).toBe(0);
});

test('a new reset replaces the timer and unmounting cancels it', () => {
  const now = 1800000000000;
  jest.useFakeTimers({now});
  const store = createStore(testState({limits: [
    {harness: 'codex', minutes: 300, used: 42, resets: now / 1000 + 30, at: now},
  ]}));
  const root = mount(store, <Limits harness="codex" />);
  act(() => store.update(state => ({...state, limits: state.limits.map(limit => ({...limit, resets: now / 1000 + 120}))})));
  expect(jest.getTimerCount()).toBe(1);
  act(() => { jest.advanceTimersByTime(30000); });
  expect(root.textContent).toContain('2m');
  unmountAll();
  expect(jest.getTimerCount()).toBe(0);
});
