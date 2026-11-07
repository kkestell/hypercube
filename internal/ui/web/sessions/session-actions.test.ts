import {afterEach, expect, mock, spyOn, test} from 'bun:test';
import {stubFetch, testBridge} from '../../test-render.tsx';
import {testSession, testState, testWorkspace} from '../../test-state.ts';
import {attachImages, canSend, discardUnsent, draftOf, typed, unsentKey as workspaceDraftKey} from '../agent/drafts.ts';
import type {Settings} from '../agent/types.ts';
import {createStore} from '../store.ts';
import {openUnsent, unsentSettingsChosen} from './selection.ts';
import {addWorkspace, chooseSession, retrySubmission, sendPrompt} from './session-actions.ts';
import type {Session} from './types.ts';
import {submissionsOf} from './submissions.ts';
import {receivedTranscript} from '../transcript/transcripts.ts';

const unsentKey = workspaceDraftKey('w');

afterEach(() => {
  mock.restore();
  delete globalThis.hypercube;
});

const idle = (id: string): Pick<Session, 'id' | 'ended' | 'activity'> => ({id, ended: false, activity: 'idle'});
const store = () => createStore(testState({selected: 's'}));
const lead: Settings = {harness: 'claude', model: 'opus', effort: 'high'};
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

test('pending and refused prompts retain images independently of a newer draft, and retry sends once', async () => {
  const first = Promise.withResolvers<Response>();
  const retry = Promise.withResolvers<Response>();
  const requests: {text: string; images: unknown[]; submission: string}[] = [];
  stubFetch((_url, init) => {
    requests.push(JSON.parse(String(init?.body)));
    return requests.length === 1 ? first.promise : retry.promise;
  });
  const s = store();
  const image = {mime_type: 'image/png', data: 'iVBORw0KGgo='};
  s.update(state => ({...state, drafts: {s: {text: 'first', images: [image], reading: 0}}}));
  const sending = sendPrompt(s, idle('s'));
  expect(draftOf(s.get(), 's').text).toBe('');
  expect(submissionsOf(s.get(), 's')[0]).toMatchObject({text: 'first', images: [image], status: 'sending'});
  s.update(state => typed(state, 's', 'second'));
  first.resolve(new Response('busy', {status: 409}));
  await sending;
  const failed = submissionsOf(s.get(), 's')[0];
  expect(failed.status).toBe('failed');
  expect(failed.error).toBe('busy');
  expect(draftOf(s.get(), 's').text).toBe('second');
  const retrying = retrySubmission(s, 's', failed.id);
  await retrySubmission(s, 's', failed.id);
  expect(requests).toHaveLength(2);
  expect(requests[1]).toMatchObject({text: 'first', images: [image]});
  expect(requests[1].submission).not.toBe(failed.id);
  retry.resolve(new Response(''));
  await retrying;
  expect(draftOf(s.get(), 's').text).toBe('second');
  expect(submissionsOf(s.get(), 's')[0].status).toBe('sent');
});

test('the unsent session cannot send until it has settings', () => {
  const s = store();
  s.update(state => typed(openUnsent(state, 'w'), unsentKey, 'hello'));
  expect(canSend(s.get(), idle(unsentKey))).toBe(false);
  s.update(state => unsentSettingsChosen(state, lead));
  expect(canSend(s.get(), idle(unsentKey))).toBe(true);
});

test('text typed while the unsent session sends moves to the created session', async () => {
  const s = store();
  s.update(state => unsentSettingsChosen(openUnsent(state, 'w'), lead));
  s.update(state => typed(state, unsentKey, 'create it'));
  const created = Promise.withResolvers<Response>();
  const requests = answer(() => created.promise);
  const sending = sendPrompt(s, idle(unsentKey));
  expect(Object.fromEntries(requests[0])).toEqual({action: 'new', workspace: 'w', harness: 'claude', model: 'opus', effort: 'high'});
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

  test(`a ${accepted ? 'successful' : 'refused'} creation preserves another workspace's new session`, async () => {
    const s = store();
    s.update(state => typed(unsentSettingsChosen(openUnsent(state, 'w'), lead), unsentKey, 'first'));
    const created = Promise.withResolvers<Response>();
    answer(() => created.promise);
    const sending = sendPrompt(s, idle(unsentKey));
    const other = workspaceDraftKey('other');
    s.update(state => typed(unsentSettingsChosen(openUnsent(state, 'other'), lead), other, 'second'));
    const replacement = s.get().unsent;
    created.resolve(reply());
    await sending;
    expect(s.get().unsent).toBe(replacement);
    expect(s.get().pending).toBeNull();
    expect(draftOf(s.get(), other).text).toBe('second');
    expect(draftOf(s.get(), unsentKey).text).toBe('');
    expect(submissionsOf(s.get(), accepted ? 'n' : unsentKey)[0]).toMatchObject({text: 'first', status: accepted ? 'sent' : 'failed'});
    if (accepted) expect(s.get().unsentDrafts[unsentKey]).toBeUndefined();
    else expect(s.get().unsentDrafts[unsentKey].sending).toBe(false);
  });

  test(`image reads do not strand a composer when creation ${accepted ? 'succeeds' : 'is refused'}`, async () => {
    const s = store();
    s.update(state => typed(unsentSettingsChosen(openUnsent(state, 'w'), lead), unsentKey, 'first'));
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
  test(`a network failure while ${phase} retains a retryable message and re-enables the unsent session`, async () => {
    answer(response);
    const s = store();
    s.update(state => typed(state, 's', 'hello'));
    await sendPrompt(s, idle('s'));
    expect(submissionsOf(s.get(), 's')[0]).toMatchObject({text: 'hello', status: 'failed'});
    expect(submissionsOf(s.get(), 's')[0].error).toContain('Load failed');
    expect(submissionsOf(s.get(), 's')[0].error).toContain('retrying may send this message again');
    expect(draftOf(s.get(), 's').text).toBe('');

    s.update(state => typed(unsentSettingsChosen(openUnsent(state, 'w'), lead), unsentKey, 'create it'));
    await sendPrompt(s, idle(unsentKey));
    expect(submissionsOf(s.get(), unsentKey)[0]).toMatchObject({text: 'create it', status: 'failed'});
    expect(s.get().unsent?.sending).toBe(false);
    expect(draftOf(s.get(), unsentKey).text).toBe('');
    s.update(state => typed(state, unsentKey, 'next'));
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

test('a failed first input transfers to the persisted session and retries there without changing selection', async () => {
  const created = Promise.withResolvers<Response>();
  const requests = answer(() => created.promise);
  const s = store();
  s.update(state => typed(unsentSettingsChosen(openUnsent(state, 'w'), lead), unsentKey, 'first'));
  const sending = sendPrompt(s, idle(unsentKey));
  s.update(state => typed(state, unsentKey, 'next'));
  s.update(state => openUnsent(state, 'other'));
  created.resolve(Response.json({session: 'n', error: 'no reply within 30s'}, {status: 409}));
  await sending;
  expect(s.get().unsent?.workspace).toBe('other');
  expect(s.get().pending).toBeNull();
  expect(draftOf(s.get(), 'n').text).toBe('next');
  const failed = submissionsOf(s.get(), 'n')[0];
  expect(failed.error).toBe('no reply within 30s');
  await retrySubmission(s, 'n', failed.id);
  expect(Object.fromEntries(requests[1])).toEqual({action: 'input', id: 'n'});
  expect(s.get().unsent?.workspace).toBe('other');
});

test('HTTP completion cannot recreate a submission acknowledged by the transcript', async () => {
  const response = Promise.withResolvers<Response>();
  answer(() => response.promise);
  const s = createStore(testState({workspaces: [testWorkspace({id: 'w', sessions: [testSession({id: 's'})]})]}));
  chooseSession(s, 's');
  s.update(state => typed(state, 's', 'same'));
  const sending = sendPrompt(s, idle('s'));
  const {id} = submissionsOf(s.get(), 's')[0];
  s.update(state => receivedTranscript(state, {type: 'transcript', session: 's', reset: true, items: [{id, seq: 1, kind: 'user', text: 'same'}]}));
  expect(submissionsOf(s.get(), 's')).toHaveLength(0);
  response.resolve(new Response(''));
  await sending;
  expect(submissionsOf(s.get(), 's')).toHaveLength(0);
});

for (const pending of [false, true]) {
  test(`discarding a ${pending ? 'pending' : 'failed'} first prompt leaves the next new session empty`, async () => {
    const response = Promise.withResolvers<Response>();
    answer(() => response.promise);
    const s = store();
    s.update(state => typed(unsentSettingsChosen(openUnsent(state, 'w'), lead), unsentKey, 'discard this'));
    const sending = sendPrompt(s, idle(unsentKey));
    if (!pending) {
      response.resolve(new Response('refused', {status: 409}));
      await sending;
    }
    s.update(state => discardUnsent(state, unsentKey));
    s.update(state => openUnsent(state, 'w'));
    expect(submissionsOf(s.get(), unsentKey)).toEqual([]);
    if (pending) {
      response.resolve(Response.json({session: 'n'}));
      await sending;
    }
    expect(submissionsOf(s.get(), unsentKey)).toEqual([]);
    expect(submissionsOf(s.get(), 'n')).toEqual([]);
    expect(s.get().unsent?.workspace).toBe('w');
    expect(s.get().pending).toBeNull();
  });
}

test('a chosen folder is added as a workspace, and a cancelled chooser posts nothing', async () => {
  const requests = answer(() => new Response('w'));
  const folders = ['/chosen folder', ''];
  globalThis.hypercube = testBridge({chooseFolder: async () => folders.shift()!});
  const target = store();
  await addWorkspace(target);
  await addWorkspace(target);
  expect(requests.map(params => Object.fromEntries(params))).toEqual([{action: 'new', path: '/chosen folder'}]);
});
