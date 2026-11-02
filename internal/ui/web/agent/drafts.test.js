import {expect, test} from 'bun:test';
import {createStore} from '../store.js';
import {attachImages, canSend, draftOf, typed, unsentCreated, unsentKey} from './drafts.js';

const lead = {id: 'lead', activity: 'idle'};
const store = (extra = {}) => createStore({workspaces: [], selected: 's', pending: null, unsent: null, drafts: {}, errors: [], session: null, ...extra});
const png = bytes => new File([new Uint8Array(bytes)], 'a.png', {type: 'image/png'});

test('attaching refuses other types and images beyond 10 MiB', async () => {
  const s = store();
  const errors = () => s.get().errors.map(error => error.message);
  await attachImages(s, 'lead', [png(4), new File(['x'], 'a.txt', {type: 'text/plain'})]);
  expect(errors()).toEqual(['Attach PNG, JPEG, GIF, or WebP images.']);
  expect(draftOf(s.get(), 'lead').images.length).toBe(1);
  await attachImages(s, 'lead', [png(10 << 20)]);
  expect(errors()).toEqual(['Images can total at most 10 MiB.', 'Attach PNG, JPEG, GIF, or WebP images.']);
  expect(draftOf(s.get(), 'lead').images.length).toBe(1);
});

test('sending waits for images still being read', async () => {
  const s = store();
  s.update(state => typed(state, 'lead', 'look'));
  const reading = attachImages(s, 'lead', [png(4)]);
  expect(canSend(s.get(), lead)).toBe(false);
  await reading;
  expect(canSend(s.get(), lead)).toBe(true);
  expect(canSend(s.get(), {...lead, activity: 'exited'})).toBe(false);
});

test('the created lead takes the unsent session\'s draft, and an empty draft leaves nothing', () => {
  const draft = {text: 'hi', images: [], reading: 1};
  const moved = unsentCreated({drafts: {[unsentKey]: draft, other: draft}}, 'n-lead');
  expect(moved.drafts).toEqual({other: draft, 'n-lead': {text: 'hi', images: [], reading: 0}});
  expect(unsentCreated({drafts: {[unsentKey]: {text: '', images: [], reading: 1}}}, 'n-lead').drafts).toEqual({});
});
