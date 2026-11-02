import {afterEach, expect, spyOn, test} from 'bun:test';
import {attachImages, canSend, draftOf, typed, unsentKey} from '../agent/drafts.js';
import {createStore} from '../store.js';
import {openUnsent, unsentProfile} from './selection.js';
import {chooseSession, sendPrompt} from './session-actions.js';

const session = {id: 's', activity: 'idle'};
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

test('a refused prompt returns unless another was started', async () => {
  const fetches = spy(globalThis, 'fetch').mockImplementation(async () => new Response('busy', {status: 409}));
  const s = store();
  s.update(state => typed(state, 's', 'first'));
  await sendPrompt(s, session);
  expect(new URL(fetches.mock.calls[0][0], 'http://x/').searchParams.get('action')).toBe('input');
  expect(draftOf(s.get(), 's').text).toBe('first');

  let release;
  fetches.mockImplementation(() => new Promise(resolve => { release = () => resolve(new Response('busy', {status: 409})); }));
  const sending = sendPrompt(s, session);
  expect(draftOf(s.get(), 's').text).toBe('');
  s.update(state => typed(state, 's', 'second'));
  release();
  await sending;
  expect(draftOf(s.get(), 's').text).toBe('second');
});

test('the unsent session cannot send until it has a profile', () => {
  const s = store();
  s.update(state => typed(openUnsent(state, 'w'), unsentKey, 'hello'));
  const unsent = {id: unsentKey, activity: 'idle'};
  expect(canSend(s.get(), unsent)).toBe(false);
  s.update(state => unsentProfile(state, {name: 'claude-lead'}));
  expect(canSend(s.get(), unsent)).toBe(true);
});

test('text typed while the unsent session sends moves to the created session', async () => {
  const s = store();
  s.update(state => unsentProfile(openUnsent(state, 'w'), {name: 'claude-lead', harness: 'claude'}));
  s.update(state => typed(state, unsentKey, 'create it'));
  let release;
  const fetches = spy(globalThis, 'fetch').mockImplementation(() => new Promise(resolve => { release = () => resolve(Response.json({session: 'n'})); }));
  const sending = sendPrompt(s, {id: unsentKey, activity: 'idle'});
  expect(Object.fromEntries(new URL(fetches.mock.calls[0][0], 'http://x/').searchParams)).toEqual({action: 'new', workspace: 'w', profile: 'claude-lead'});
  expect(s.get().unsent.sending).toBe(true);
  s.update(state => typed(state, unsentKey, 'next'));
  release();
  await sending;
  expect(s.get().unsent).toBeNull();
  expect(s.get().pending).toBe('n');
  expect(s.get().drafts).toEqual({'n': {text: 'next', images: [], reading: 0}});
});

for (const accepted of [true, false]) {
  test(`a ${accepted ? 'successful' : 'refused'} creation leaves a replacement unsent session alone`, async () => {
    const s = store();
    const profile = {name: 'claude-lead', harness: 'claude'};
    s.update(state => typed(unsentProfile(openUnsent(state, 'w'), profile), unsentKey, 'first'));
    let release;
    spy(globalThis, 'fetch').mockImplementation(() => new Promise(resolve => { release = resolve; }));
    const sending = sendPrompt(s, {id: unsentKey, activity: 'idle'});
    s.update(state => typed(unsentProfile(openUnsent(state, 'w'), profile), unsentKey, 'second'));
    const replacement = s.get().unsent;
    release(accepted ? Response.json({session: 'n'}) : new Response('refused', {status: 409}));
    await sending;
    expect(s.get().unsent).toBe(replacement);
    expect(s.get().pending).toBeNull();
    expect(s.get().drafts).toEqual({[unsentKey]: {text: 'second', images: [], reading: 0}});
  });

  test(`image reads do not strand a composer when creation ${accepted ? 'succeeds' : 'is refused'}`, async () => {
    const s = store();
    s.update(state => typed(unsentProfile(openUnsent(state, 'w'), {name: 'claude-lead'}), unsentKey, 'first'));
    let release, reader;
    spy(globalThis, 'fetch').mockImplementation(() => new Promise(resolve => { release = resolve; }));
    spy(FileReader.prototype, 'readAsDataURL').mockImplementation(function() { reader = this; });
    const sending = sendPrompt(s, {id: unsentKey, activity: 'idle'});
    s.update(state => typed(state, unsentKey, 'next'));
    const reading = attachImages(s, unsentKey, [png(4)]);
    release(accepted ? Response.json({session: 'n'}) : new Response('refused', {status: 409}));
    await sending;
    Object.defineProperty(reader, 'result', {value: 'data:image/png;base64,AAAAAA=='});
    reader.onload();
    await reading;
    const key = accepted ? 'n' : unsentKey;
    expect(draftOf(s.get(), key).reading).toBe(0);
    expect(draftOf(s.get(), key).images.length).toBe(accepted ? 0 : 1);
    expect(canSend(s.get(), {id: key, activity: 'idle'})).toBe(true);
    if (accepted) expect(s.get().drafts[unsentKey]).toBeUndefined();
  });
}

for (const [phase, response] of [
  ['fetching', () => Promise.reject(new TypeError('Load failed'))],
  ['reading the response', () => Promise.resolve(new Response(new ReadableStream({
    start(controller) { controller.error(new TypeError('Load failed')); },
  })))],
]) {
  test(`a network failure while ${phase} shows an error, keeps the draft, and re-enables the unsent session`, async () => {
    spy(globalThis, 'fetch').mockImplementation(response);
    const s = store();
    s.update(state => typed(state, 's', 'hello'));
    await sendPrompt(s, session);
    expect(s.get().errors).toHaveLength(1);
    expect(s.get().errors[0].message).toContain('Load failed');
    expect(draftOf(s.get(), 's').text).toBe('hello');

    s.update(state => typed(unsentProfile(openUnsent(state, 'w'), {name: 'claude-lead'}), unsentKey, 'create it'));
    await sendPrompt(s, {id: unsentKey, activity: 'idle'});
    expect(s.get().errors.length).toBe(2);
    expect(s.get().unsent.sending).toBe(false);
    expect(draftOf(s.get(), unsentKey).text).toBe('create it');
    expect(canSend(s.get(), {id: unsentKey, activity: 'idle'})).toBe(true);
  });
}

test('choosing a stopped child starts its parent, and choosing an ended child only selects', () => {
  const fetches = spyOn(globalThis, 'fetch').mockImplementation(async () => new Response(''));
  try {
    const child = {id: 'c', parent: 'p', children: [], running: false};
    const s = createStore({workspaces: [{id: 'w', sessions: [{id: 'p', children: [child], running: false}]}], drafts: {}, selected: null, pending: null, session: null, unsent: null});
    chooseSession(s, 'c');
    expect(s.get().session.id).toBe('c');
    expect(new URL(fetches.mock.calls[0][0], 'http://x/').searchParams.get('id')).toBe('p');
    s.update(state => ({...state, workspaces: [{id: 'w', sessions: [{id: 'p', children: [{...child, ended: true}], running: true}]}]}));
    chooseSession(s, 'c');
    expect(fetches.mock.calls.length).toBe(1);
  } finally { fetches.mockRestore(); }
});
