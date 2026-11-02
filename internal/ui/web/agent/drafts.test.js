import {afterEach, expect, spyOn, test} from 'bun:test';
import {createStore} from '../store.js';
import {unsentKey} from '../sessions/selection.js';
import {attachImages, canSend, draftOf, sendPrompt, typed} from './drafts.js';

const lead = {id: 'lead', activity: 'idle'};
const store = (extra = {}) => createStore({workspaces: [], selected: 's', pending: null, unsent: null, drafts: {}, session: null, ...extra});
const png = bytes => new File([new Uint8Array(bytes)], 'a.png', {type: 'image/png'});

let spies = [];
const spy = (object, method) => {
  const s = spyOn(object, method);
  spies.push(s);
  return s;
};
afterEach(() => {
  for (const s of spies) s.mockRestore();
  spies = [];
});

test('attaching refuses other types and images beyond 10 MiB', async () => {
  const alerts = spy(globalThis, 'alert').mockImplementation(() => {});
  const s = store();
  await attachImages(s, 'lead', [png(4), new File(['x'], 'a.txt', {type: 'text/plain'})]);
  expect(alerts.mock.calls).toEqual([['Attach PNG, JPEG, GIF, or WebP images.']]);
  expect(draftOf(s.get(), 'lead').images.length).toBe(1);
  await attachImages(s, 'lead', [png(10 << 20)]);
  expect(alerts.mock.calls[1]).toEqual(['Images can total at most 10 MiB.']);
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

test('a refused prompt returns unless another was started', async () => {
  spy(globalThis, 'alert').mockImplementation(() => {});
  const fetches = spy(globalThis, 'fetch').mockImplementation(async () => new Response('busy', {status: 409}));
  const s = store();
  s.update(state => typed(state, 'lead', 'first'));
  await sendPrompt(s, lead);
  expect(new URL(fetches.mock.calls[0][0], 'http://x/').searchParams.get('action')).toBe('input');
  expect(draftOf(s.get(), 'lead').text).toBe('first');

  let release;
  fetches.mockImplementation(() => new Promise(resolve => { release = () => resolve(new Response('busy', {status: 409})); }));
  const sending = sendPrompt(s, lead);
  expect(draftOf(s.get(), 'lead').text).toBe('');
  s.update(state => typed(state, 'lead', 'second'));
  release();
  await sending;
  expect(draftOf(s.get(), 'lead').text).toBe('second');
});

test('text typed while the unsent session sends moves to the created lead', async () => {
  const unsent = {workspace: 'w', lead: 'claude-lead', harness: 'claude', sending: false};
  const s = store({unsent});
  s.update(state => typed(state, unsentKey, 'create it'));
  let release;
  spy(globalThis, 'fetch').mockImplementation(() => new Promise(resolve => { release = () => resolve(Response.json({session: 'n', lead: 'n-lead'})); }));
  const sending = sendPrompt(s, {id: unsentKey, activity: 'idle'});
  expect(s.get().unsent.sending).toBe(true);
  s.update(state => typed(state, unsentKey, 'next'));
  release();
  await sending;
  expect(s.get().unsent).toBeNull();
  expect(s.get().pending).toBe('n');
  expect(s.get().drafts).toEqual({'n-lead': {text: 'next', images: [], reading: 0}});
});
