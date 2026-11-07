import {afterEach, expect, mock, test} from 'bun:test';
import {act} from 'react';
import {find, findAll, mount, stubFetch, unmountAll} from '../../test-render.tsx';
import {testConfig, testSession, testState, testWorkspace} from '../../test-state.ts';
import {AgentPane} from '../agent/agent-pane.tsx';
import {draftOf, typed, unsentKey} from '../agent/drafts.ts';
import {emptySession, openUnsent, receivedWorkspaces, selectSession, shownSession} from '../sessions/selection.ts';
import {retrySubmission, sendPrompt} from '../sessions/session-actions.ts';
import {submissionsOf} from '../sessions/submissions.ts';
import {createStore} from '../store.ts';
import {Transcript} from './transcript.tsx';
import {displayedTranscript, receivedTranscript} from './transcripts.ts';

afterEach(() => { mock.restore(); unmountAll(); });

const image = {mime_type: 'image/png', data: 'iVBORw0KGgo='};
const session = testSession({id: 's'});
const workspace = testWorkspace({id: 'w', sessions: [session, testSession({id: 'other'})]});

for (const fresh of [false, true]) {
  test(`${fresh ? 'first' : 'follow-up'} prompt shows text and images before HTTP completes, with a working retry`, async () => {
    const first = Promise.withResolvers<Response>();
    const retry = Promise.withResolvers<Response>();
    const requests: {text: string; images: unknown[]; submission: string}[] = [];
    stubFetch((url, init) => {
      if (url.startsWith('models?')) return Response.json({models: []});
      requests.push(JSON.parse(String(init?.body)));
      return requests.length === 1 ? first.promise : retry.promise;
    });
    let initial = testState({config: testConfig(), workspaces: [workspace], selected: 's', session: emptySession('s')});
    if (fresh) initial = openUnsent(initial, 'w');
    const store = createStore(initial);
    const shown = shownSession(initial)!;
    const key = shown.id;
    store.update(state => ({...state, drafts: {[key]: {text: 'Inspect this', images: [image], reading: 0}}}));
    const root = mount(store, <AgentPane session={shown} active />);
    let sending: Promise<void>;
    act(() => { sending = sendPrompt(store, shown); });
    expect(root.querySelector('textarea')!.value).toBe('');
    expect(find(root, '.message-item-user').textContent).toContain('Inspect this');
    expect(find(root, '.message-item-user img').getAttribute('src')).toBe('data:image/png;base64,' + image.data);
    expect(find(root, '[role=status]').textContent).toBe('Sending…');
    act(() => store.update(state => typed(state, key, 'Next draft')));
    await act(async () => { first.resolve(new Response('busy', {status: 409})); await sending!; });
    expect(find(root, '[role=alert]').textContent).toBe('Sending failed: busy');
    expect(draftOf(store.get(), key).text).toBe('Next draft');
    act(() => {
      findAll(root, 'button').find(button => button.textContent === 'Retry')!.click();
      findAll(root, 'button').find(button => button.textContent === 'Retry')?.click();
    });
    expect(requests).toHaveLength(2);
    expect(requests[1]).toMatchObject({text: 'Inspect this', images: [image]});
    expect(findAll(root, '.message-item-user')).toHaveLength(1);
    expect(root.querySelector('[role=alert]')).toBeNull();
    // A definitive second refusal keeps the original message retryable.
    await act(async () => { retry.resolve(new Response('still busy', {status: 409})); });
    expect(find(root, '[role=alert]').textContent).toContain('still busy');
    expect(draftOf(store.get(), key).text).toBe('Next draft');
  });
}

for (const reset of [false, true]) {
  test(`identical prompts reconcile by ID on a ${reset ? 'reset' : 'incremental update'}, in either response order`, async () => {
    const first = Promise.withResolvers<Response>();
    const second = Promise.withResolvers<Response>();
    let requests = 0;
    stubFetch(() => ++requests === 1 ? first.promise : second.promise);
    const store = createStore(testState({workspaces: [workspace], selected: 's', session: emptySession('s')}));
    const root = mount(store, <Transcript session={session} />);
    const send = () => {
      store.update(state => typed(state, 's', 'same'));
      return sendPrompt(store, session);
    };
    let sendingFirst: Promise<void>, sendingSecond: Promise<void>;
    act(() => { sendingFirst = send(); sendingSecond = send(); });
    expect(findAll(root, '.message-item-user')).toHaveLength(2);
    const [one, two] = submissionsOf(store.get(), 's');
    // One HTTP response precedes the transcript, and the other follows it.
    await act(async () => { second.resolve(new Response('')); await sendingSecond!; });
    act(() => store.update(state => receivedTranscript(state, {type: 'transcript', session: 's', reset, items: [
      {id: one.id, seq: 1, kind: 'user', text: 'same'},
      {id: two.id, seq: 2, kind: 'user', text: 'same'},
    ]})));
    await act(async () => { first.resolve(new Response('')); await sendingFirst!; });
    expect(findAll(root, '.message-item-user')).toHaveLength(2);
    expect(submissionsOf(store.get(), 's')).toHaveLength(0);
    expect(root.querySelector('[role=status]')).toBeNull();
  });
}

test('switching sessions retains a failed message, and removing its session clears it', async () => {
  stubFetch(() => new Response('refused', {status: 409}));
  const store = createStore(testState({workspaces: [workspace], selected: 's', session: emptySession('s')}));
  store.update(state => typed(state, 's', 'Retain this'));
  await sendPrompt(store, session);
  store.update(state => selectSession(state, 'other'));
  expect(displayedTranscript(store.get(), 'other').order).toHaveLength(0);
  store.update(state => selectSession(state, 's'));
  expect(displayedTranscript(store.get(), 's').order).toHaveLength(1);
  store.update(state => receivedWorkspaces(state, [{...workspace, sessions: [testSession({id: 'other'})]}]));
  expect(submissionsOf(store.get(), 's')).toHaveLength(0);
});

test('a failed unsent message survives parking and retry uses its retained prompt', async () => {
  const requests: string[] = [];
  stubFetch((_url, init) => { requests.push(String(init?.body)); return new Response('refused', {status: 409}); });
  const store = createStore(openUnsent(testState({config: testConfig(), workspaces: [workspace]}), 'w'));
  const key = unsentKey('w');
  store.update(state => typed(state, key, 'First prompt'));
  await sendPrompt(store, shownSession(store.get())!);
  store.update(state => selectSession(state, 's'));
  expect(store.get().unsentDrafts[key]).toBeDefined();
  store.update(state => openUnsent(state, 'w'));
  const failed = submissionsOf(store.get(), key)[0];
  expect(failed.text).toBe('First prompt');
  await retrySubmission(store, key, failed.id);
  expect(JSON.parse(requests[1]).text).toBe('First prompt');
});

test('the first prompt follows its end and lets the user scroll back before a transcript exists', async () => {
  const response = Promise.withResolvers<Response>();
  stubFetch(url => url.startsWith('models?') ? Response.json({models: []}) : response.promise);
  const store = createStore(openUnsent(testState({config: testConfig(), workspaces: [workspace]}), 'w'));
  const shown = shownSession(store.get())!;
  store.update(state => typed(state, shown.id, 'A long first prompt.\n'.repeat(100)));
  const sending = sendPrompt(store, shown);
  const root = mount(store, <AgentPane session={shown} active />);
  const view = find(root, '.transcript');
  // Happy DOM has no layout: model a viewport with a long first message.
  let top = 0;
  Object.defineProperties(view, {
    scrollTop: {get: () => top, set: (value: number) => { top = Math.max(0, Math.min(value, 900)); }},
    scrollHeight: {get: () => 1000},
    clientHeight: {get: () => 100},
  });
  act(() => store.update(state => typed(state, shown.id, 'Next draft')));
  expect(view.scrollTop).toBe(900);
  view.scrollTop = 200;
  act(() => view.dispatchEvent(new Event('scroll')));
  act(() => store.update(state => typed(state, shown.id, 'More draft')));
  expect(view.scrollTop).toBe(200);
  act(() => find(root, '[aria-label="Scroll to newest output"]').click());
  expect(view.scrollTop).toBe(900);
  await act(async () => { response.resolve(new Response('refused', {status: 409})); await sending; });
});
