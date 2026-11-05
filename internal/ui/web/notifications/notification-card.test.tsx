import {afterEach, expect, jest, mock, spyOn, test} from 'bun:test';
import {act} from 'react';
import {clickControl, find, mount, unmountAll} from '../../test-render.tsx';
import {testSession, testState, testWorkspace} from '../../test-state.ts';
import {post} from '../api.ts';
import {createStore} from '../store.ts';
import {NotificationStack} from './notification-stack.tsx';

afterEach(() => {
  jest.useRealTimers();
  mock.restore();
  unmountAll();
});

const workspaces = [testWorkspace({id: 'w', name: 'repo', sessions: [
  testSession({id: 'a', title: 'Fix it', unseen: true, reply: 'Done.\nAll tests pass.', replySeq: 4}),
  testSession({id: 'b', title: 'Ask', children: [testSession({id: 'b-w', parent: 'b', title: 'auditor'})]}),
]})];
const state = testState({
  workspaces,
  requests: [{session: 'b-w', item: {id: 'q', kind: 'attention', text: 'Allow?', choices: ['Yes']}}],
});


test('cards show their source and request answers', () => {
  const root = mount(createStore(state), <NotificationStack />);
  const [request, reply] = root.querySelectorAll('.notification-card');
  expect(request.querySelector('.notification-card-header .notification-card-session')?.textContent).toBe('auditor');
  expect([...request.querySelectorAll('.notification-card-context span')].map(span => span.textContent)).toEqual(['repo', 'Ask']);
  expect(request.querySelector('button[data-choice]')?.textContent).toBe('Yes');
  expect(reply.querySelector('.notification-card-header .notification-card-session')?.textContent).toBe('Fix it');
  expect(reply.querySelector('.notification-card-context')?.textContent).toBe('repo');
  expect(reply.querySelector('.notification-card-text')?.textContent).toBe('Done.\nAll tests pass.');
});

test('a reply preview renders inline Markdown and flattens blocks', () => {
  const reply = [
    '# Heading',
    '',
    'This is a **test paragraph** with some *markdown*, `code`, and a [link](https://example.com).',
    '',
    '- first',
    '- second',
    '',
    '> quoted',
    '',
    '```js',
    'let a = 1;',
    '```',
    '',
    '![picture](https://example.com/picture.png)',
  ].join('\n');
  const replyState = {...state, requests: [], workspaces: [testWorkspace({id: 'w', name: 'repo', sessions: [
    testSession({id: 'a', title: 'Fix it', unseen: true, reply, replySeq: 1}),
  ]})]};
  const root = mount(createStore(replyState), <NotificationStack />);
  const preview = find(root, '.notification-card-text');

  expect(preview.classList.contains('notification-card-reply')).toBe(true);
  expect(preview.querySelector('strong')?.textContent).toBe('test paragraph');
  expect(preview.querySelector('em')?.textContent).toBe('markdown');
  expect(preview.querySelector('code')?.textContent).toBe('code');
  expect(preview.querySelector('a')?.getAttribute('href')).toBe('https://example.com/');
  for (const tag of ['h1', 'ul', 'blockquote', 'pre', 'img']) expect(preview.querySelector(tag)).toBeNull();
  expect(preview.textContent).toBe([
    'Heading',
    'This is a test paragraph with some markdown, code, and a link.',
    'first',
    'second',
    'quoted',
    'let a = 1;',
    'picture',
  ].join('\n'));
});

test('closing a reply card leaves the reply unseen', () => {
  const fetches = spyOn(globalThis, 'fetch').mockResolvedValue(new Response(''));
  const store = createStore(state);
  const root = mount(store, <NotificationStack />);
  act(() => root.querySelectorAll<HTMLElement>('[title="Close"]')[1].click());
  expect(root.querySelectorAll('.notification-card').length).toBe(1);
  expect(store.get().workspaces[0].sessions[0].unseen).toBe(true);
  expect(fetches).not.toHaveBeenCalled();
});

test('View Session shows the card\'s session', () => {
  const fetches = spyOn(globalThis, 'fetch').mockResolvedValue(new Response(''));
  const store = createStore(state);
  const root = mount(store, <NotificationStack />);
  act(() => root.querySelectorAll<HTMLElement>('[title="View Session"]')[1].click());
  expect(store.get().selected).toBe('a');
  expect(root.querySelectorAll('.notification-card').length).toBe(1);
});

test('a reply card closes after 15 seconds and leaves the reply unseen', () => {
  jest.useFakeTimers();
  const fetches = spyOn(globalThis, 'fetch').mockResolvedValue(new Response(''));
  const store = createStore(state);
  const root = mount(store, <NotificationStack />);
  act(() => jest.advanceTimersByTime(14999));
  expect(root.querySelectorAll('.notification-card').length).toBe(2);
  act(() => jest.advanceTimersByTime(1));
  expect(root.querySelectorAll('.notification-card').length).toBe(1);
  expect(store.get().dismissed).toEqual({'reply:a:4': true});
  expect(store.get().workspaces[0].sessions[0].unseen).toBe(true);
  expect(fetches).not.toHaveBeenCalled();
});

test('a refused action shows an error card, newest first, until it is closed', async () => {
  const fetches = spyOn(globalThis, 'fetch').mockResolvedValue(new Response('session is busy', {status: 409}));
  const store = createStore({...state, workspaces: [], requests: []});
  const root = mount(store, <NotificationStack />);
  expect(await post(store, 'session', {action: 'stop', id: 'a'})).toBeNull();
  fetches.mockResolvedValue(new Response('no such session', {status: 404}));
  await post(store, 'session', {action: 'stop', id: 'b'});
  const texts = () => [...root.querySelectorAll('.notification-card')].map(card => card.textContent);
  expect(texts()).toEqual(['no such session', 'session is busy']);
  expect(root.querySelector('[title="View Session"]')).toBeNull();
  act(() => clickControl(find(root, '[title="Close"]')));
  expect(texts()).toEqual(['session is busy']);
});
