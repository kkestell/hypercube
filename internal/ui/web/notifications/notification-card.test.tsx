import {afterEach, expect, jest, mock, spyOn, test} from 'bun:test';
import {act} from 'react';
import {find, mount, unmountAll} from '../../test-render.tsx';
import {testSession, testState, testWorkspace} from '../../test-state.ts';
import {createStore} from '../store.ts';
import {NotificationCard} from './notification-card.tsx';
import {cards} from './notifications.ts';

afterEach(() => {
  unmountAll();
  jest.useRealTimers();
  mock.restore();
});

test('reply expiry pauses for hover and focus, then resumes its remaining lifetime', () => {
  jest.useFakeTimers();
  spyOn(performance, 'now').mockImplementation(() => Date.now());
  const session = testSession({id: 's', unseen: true, reply: 'Finished', replySeq: 2});
  const store = createStore(testState({workspaces: [testWorkspace({id: 'w', sessions: [session]})]}));
  const card = cards(store.get())[0];
  const root = mount(store, <NotificationCard card={card} />);
  const notification = find(root, '[role=alert]');
  const advance = (ms: number) => act(() => jest.advanceTimersByTime(ms));
  advance(5000);
  act(() => notification.dispatchEvent(new PointerEvent('pointerenter')));
  advance(20000);
  expect(cards(store.get())).toHaveLength(1);
  act(() => find(root, '[aria-label=Close]').focus());
  act(() => notification.dispatchEvent(new PointerEvent('pointerleave')));
  advance(20000);
  expect(cards(store.get())).toHaveLength(1);
  act(() => find(root, '[aria-label="View Session"]').focus());
  advance(20000);
  expect(cards(store.get())).toHaveLength(1);
  act(() => find(root, '[aria-label="View Session"]').blur());
  advance(9999);
  expect(cards(store.get())).toHaveLength(1);
  advance(1);
  expect(cards(store.get())).toEqual([]);
  expect(store.get().workspaces[0].sessions[0].unseen).toBe(true);
});

test('close remains available while reply expiry is paused', () => {
  const store = createStore(testState({workspaces: [testWorkspace({id: 'w', sessions: [testSession({id: 's', unseen: true, reply: 'Done', replySeq: 1})]})]}));
  const root = mount(store, <NotificationCard card={cards(store.get())[0]} />);
  act(() => find(root, '[role=alert]').dispatchEvent(new PointerEvent('pointerenter')));
  act(() => find(root, '[aria-label=Close]').click());
  expect(cards(store.get())).toEqual([]);
  expect(store.get().workspaces[0].sessions[0].unseen).toBe(true);
});
