import {expect, test} from 'bun:test';
import {createStore} from '../store.ts';
import {testSession, testState} from '../../test-state.ts';
import type {State} from '../state.ts';
import {attachImages, canSend, draftOf, typed, unsentCreated, unsentKey} from './drafts.ts';

const session = testSession({id: 's'});
const store = (extra: Partial<State> = {}) => createStore(testState({selected: 's', ...extra}));
const png = (bytes: number) => new File([new Uint8Array(bytes)], 'a.png', {type: 'image/png'});

test('attaching refuses other types and images beyond 10 MiB', async () => {
  const s = store();
  const errors = () => s.get().errors.map(error => error.message);
  await attachImages(s, 's', [png(4), new File(['x'], 'a.txt', {type: 'text/plain'})]);
  expect(errors()).toEqual(['Attach PNG, JPEG, GIF, or WebP images.']);
  expect(draftOf(s.get(), 's').images.length).toBe(1);
  await attachImages(s, 's', [png(10 << 20)]);
  expect(errors()).toEqual(['Images can total at most 10 MiB.', 'Attach PNG, JPEG, GIF, or WebP images.']);
  expect(draftOf(s.get(), 's').images.length).toBe(1);
});

test('sending waits for images still being read', async () => {
  const s = store();
  s.update(state => typed(state, 's', 'look'));
  const reading = attachImages(s, 's', [png(4)]);
  expect(canSend(s.get(), session)).toBe(false);
  await reading;
  expect(canSend(s.get(), session)).toBe(true);
  expect(canSend(s.get(), {...session, activity: 'exited'})).toBe(false);
});

test('the created session takes the unsent session\'s draft, and an empty draft leaves nothing', () => {
  const draft = {text: 'hi', images: [], reading: 1};
  const moved = unsentCreated(testState({drafts: {[unsentKey]: draft, other: draft}}), 'n');
  expect(moved.drafts).toEqual({other: draft, 'n': {text: 'hi', images: [], reading: 0}});
  expect(unsentCreated(testState({drafts: {[unsentKey]: {text: '', images: [], reading: 1}}}), 'n').drafts).toEqual({});
});
