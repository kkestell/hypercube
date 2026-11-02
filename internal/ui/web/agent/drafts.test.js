import {afterEach, expect, spyOn, test} from 'bun:test';
import {createStore} from '../store.js';
import {openUnsent, unsentKey} from '../sessions/selection.js';
import {attachImages, canSend, draftOf, sendPrompt, typed} from './drafts.js';

const lead = {id: 'lead', activity: 'idle'};
const store = (extra = {}) => createStore({workspaces: [], selected: 's', pending: null, unsent: null, drafts: {}, errors: [], session: null, ...extra});
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

test('a refused prompt returns unless another was started', async () => {
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
  const s = store();
  s.update(state => openUnsent(state, 'w', {name: 'claude-lead', harness: 'claude'}));
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

for (const accepted of [true, false]) {
  test(`a ${accepted ? 'successful' : 'refused'} creation leaves a replacement unsent session alone`, async () => {
      const s = store();
    const profile = {name: 'claude-lead', harness: 'claude'};
    s.update(state => typed(openUnsent(state, 'w', profile), unsentKey, 'first'));
    let release;
    spy(globalThis, 'fetch').mockImplementation(() => new Promise(resolve => { release = resolve; }));
    const sending = sendPrompt(s, {id: unsentKey, activity: 'idle'});
    s.update(state => typed(openUnsent(state, 'w', profile), unsentKey, 'second'));
    const replacement = s.get().unsent;
    release(accepted ? Response.json({session: 'n', lead: 'n-lead'}) : new Response('refused', {status: 409}));
    await sending;
    expect(s.get().unsent).toBe(replacement);
    expect(s.get().pending).toBeNull();
    expect(s.get().drafts).toEqual({[unsentKey]: {text: 'second', images: [], reading: 0}});
  });

  test(`image reads do not strand a composer when creation ${accepted ? 'succeeds' : 'is refused'}`, async () => {
      const s = store();
    s.update(state => typed(openUnsent(state, 'w', {name: 'claude-lead'}), unsentKey, 'first'));
    let release, reader;
    spy(globalThis, 'fetch').mockImplementation(() => new Promise(resolve => { release = resolve; }));
    spy(FileReader.prototype, 'readAsDataURL').mockImplementation(function() { reader = this; });
    const sending = sendPrompt(s, {id: unsentKey, activity: 'idle'});
    s.update(state => typed(state, unsentKey, 'next'));
    const reading = attachImages(s, unsentKey, [png(4)]);
    release(accepted ? Response.json({session: 'n', lead: 'n-lead'}) : new Response('refused', {status: 409}));
    await sending;
    Object.defineProperty(reader, 'result', {value: 'data:image/png;base64,AAAAAA=='});
    reader.onload();
    await reading;
    const key = accepted ? 'n-lead' : unsentKey;
    expect(draftOf(s.get(), key).reading).toBe(0);
    expect(draftOf(s.get(), key).images.length).toBe(accepted ? 0 : 1);
    expect(canSend(s.get(), {id: key, activity: 'idle'})).toBe(true);
    if (accepted) expect(s.get().drafts[unsentKey]).toBeUndefined();
  });
}
