import {afterEach, expect, mock, spyOn, test} from 'bun:test';
import {stubFetch} from '../../test-render.tsx';
import {testSession, testState, testWorkspace} from '../../test-state.ts';
import {attachImages, canSend, draftOf, typed, unsentKey} from '../agent/drafts.ts';
import type {Profile} from '../agent/types.ts';
import {createStore} from '../store.ts';
import {openUnsent, unsentProfile} from './selection.ts';
import {chooseSession, sendPrompt} from './session-actions.ts';
import type {Session} from './types.ts';

afterEach(() => mock.restore());

const idle = (id: string): Pick<Session, 'id' | 'ended' | 'activity'> => ({id, ended: false, activity: 'idle'});
const store = () => createStore(testState({selected: 's'}));
const lead: Profile = {name: 'claude-lead', description: '', harness: 'claude', model: 'opus', effort: 'high'};
const png = (bytes: number) => new File([new Uint8Array(bytes)], 'a.png', {type: 'image/png'});

// answer replies to every request with respond(), and returns the query
// parameters of the requests as they arrive.
function answer(respond: () => Response | Promise<Response>) {
  const requests: URLSearchParams[] = [];
  stubFetch(url => {
    requests.push(new URL(url, 'http://x/').searchParams);
    return respond();
  });
  return requests;
}

test('a refused prompt returns unless another was started', async () => {
  let respond: () => Response | Promise<Response> = () => new Response('busy', {status: 409});
  const requests = answer(() => respond());
  const s = store();
  s.update(state => typed(state, 's', 'first'));
  await sendPrompt(s, idle('s'));
  expect(requests[0].get('action')).toBe('input');
  expect(draftOf(s.get(), 's').text).toBe('first');

  const refusal = Promise.withResolvers<Response>();
  respond = () => refusal.promise;
  const sending = sendPrompt(s, idle('s'));
  expect(draftOf(s.get(), 's').text).toBe('');
  s.update(state => typed(state, 's', 'second'));
  refusal.resolve(new Response('busy', {status: 409}));
  await sending;
  expect(draftOf(s.get(), 's').text).toBe('second');
});

test('the unsent session cannot send until it has a profile', () => {
  const s = store();
  s.update(state => typed(openUnsent(state, 'w'), unsentKey, 'hello'));
  expect(canSend(s.get(), idle(unsentKey))).toBe(false);
  s.update(state => unsentProfile(state, lead));
  expect(canSend(s.get(), idle(unsentKey))).toBe(true);
});

test('text typed while the unsent session sends moves to the created session', async () => {
  const s = store();
  s.update(state => unsentProfile(openUnsent(state, 'w'), lead));
  s.update(state => typed(state, unsentKey, 'create it'));
  const created = Promise.withResolvers<Response>();
  const requests = answer(() => created.promise);
  const sending = sendPrompt(s, idle(unsentKey));
  expect(Object.fromEntries(requests[0])).toEqual({action: 'new', workspace: 'w', profile: 'claude-lead'});
  expect(s.get().unsent?.sending).toBe(true);
  s.update(state => typed(state, unsentKey, 'next'));
  created.resolve(Response.json({session: 'n'}));
  await sending;
  expect(s.get().unsent).toBeNull();
  expect(s.get().pending).toBe('n');
  expect(s.get().drafts).toEqual({'n': {text: 'next', images: [], reading: 0}});
});

for (const accepted of [true, false]) {
  const reply = () => accepted ? Response.json({session: 'n'}) : new Response('refused', {status: 409});

  test(`a ${accepted ? 'successful' : 'refused'} creation leaves a replacement unsent session alone`, async () => {
    const s = store();
    s.update(state => typed(unsentProfile(openUnsent(state, 'w'), lead), unsentKey, 'first'));
    const created = Promise.withResolvers<Response>();
    answer(() => created.promise);
    const sending = sendPrompt(s, idle(unsentKey));
    s.update(state => typed(unsentProfile(openUnsent(state, 'w'), lead), unsentKey, 'second'));
    const replacement = s.get().unsent;
    created.resolve(reply());
    await sending;
    expect(s.get().unsent).toBe(replacement);
    expect(s.get().pending).toBeNull();
    expect(s.get().drafts).toEqual({[unsentKey]: {text: 'second', images: [], reading: 0}});
  });

  test(`image reads do not strand a composer when creation ${accepted ? 'succeeds' : 'is refused'}`, async () => {
    const s = store();
    s.update(state => typed(unsentProfile(openUnsent(state, 'w'), lead), unsentKey, 'first'));
    const created = Promise.withResolvers<Response>();
    const readers: FileReader[] = [];
    answer(() => created.promise);
    spyOn(FileReader.prototype, 'readAsDataURL').mockImplementation(function (this: FileReader) { readers.push(this); });
    const sending = sendPrompt(s, idle(unsentKey));
    s.update(state => typed(state, unsentKey, 'next'));
    const reading = attachImages(s, unsentKey, [png(4)]);
    created.resolve(reply());
    await sending;
    Object.defineProperty(readers[0], 'result', {value: 'data:image/png;base64,AAAAAA=='});
    readers[0].dispatchEvent(new ProgressEvent('load'));
    await reading;
    const key = accepted ? 'n' : unsentKey;
    expect(draftOf(s.get(), key).reading).toBe(0);
    expect(draftOf(s.get(), key).images.length).toBe(accepted ? 0 : 1);
    expect(canSend(s.get(), idle(key))).toBe(true);
    if (accepted) expect(s.get().drafts[unsentKey]).toBeUndefined();
  });
}

const failures: [string, () => Promise<Response>][] = [
  ['fetching', () => Promise.reject(new TypeError('Load failed'))],
  ['reading the response', () => Promise.resolve(new Response(new ReadableStream({
    start(controller) { controller.error(new TypeError('Load failed')); },
  })))],
];
for (const [phase, response] of failures) {
  test(`a network failure while ${phase} shows an error, keeps the draft, and re-enables the unsent session`, async () => {
    answer(response);
    const s = store();
    s.update(state => typed(state, 's', 'hello'));
    await sendPrompt(s, idle('s'));
    expect(s.get().errors).toHaveLength(1);
    expect(s.get().errors[0].message).toContain('Load failed');
    expect(draftOf(s.get(), 's').text).toBe('hello');

    s.update(state => typed(unsentProfile(openUnsent(state, 'w'), lead), unsentKey, 'create it'));
    await sendPrompt(s, idle(unsentKey));
    expect(s.get().errors.length).toBe(2);
    expect(s.get().unsent?.sending).toBe(false);
    expect(draftOf(s.get(), unsentKey).text).toBe('create it');
    expect(canSend(s.get(), idle(unsentKey))).toBe(true);
  });
}

test('choosing a session only selects it', () => {
  const requests = answer(() => new Response(''));
  const child = testSession({id: 'c', parent: 'p', activity: 'exited'});
  const s = createStore(testState({workspaces: [testWorkspace({id: 'w', sessions: [testSession({id: 'p', activity: 'exited', children: [child]})]})]}));
  chooseSession(s, 'c');
  expect(s.get().session?.id).toBe('c');
  expect(requests.length).toBe(0);
});
