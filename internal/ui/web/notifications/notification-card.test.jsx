import {afterEach, expect, jest, spyOn, test} from 'bun:test';
import {act} from 'preact/test-utils';
import {mount, unmountAll} from '../../test-render.jsx';
import {post} from '../api.js';
import {createStore} from '../store.js';
import {NotificationStack} from './notification-stack.jsx';

afterEach(() => {
  jest.useRealTimers();
  unmountAll();
});

const workspaces = [{id: 'w', name: 'repo', sessions: [
  {id: 'a', title: 'Fix it', unseen: true, reply: 'Done.\nAll tests pass.', replySeq: 4, children: []},
  {id: 'b', title: 'Ask', unseen: false, children: [{id: 'b-w', parent: 'b', title: 'auditor', children: []}]},
]}];
const requests = [{session: 'b-w', item: {id: 'q', kind: 'attention', text: 'Allow?', choices: ['Yes']}}];
const state = {workspaces, requests, selected: '', pending: null, unsent: null, drafts: {}, dismissed: {}, errors: [], session: null, overlays: {}};

test('cards show their source and request answers', () => {
  const root = mount(createStore(state), <NotificationStack />);
  const [request, reply] = root.querySelectorAll('.notification-card');
  expect(request.querySelector('.notification-card-source').textContent).toBe('repo — Ask — auditor');
  expect(request.querySelector('vscode-button').textContent).toBe('Yes');
  expect(reply.querySelector('.notification-card-source').textContent).toBe('repo — Fix it');
  expect(reply.querySelector('.notification-card-text').textContent).toBe('Done.\nAll tests pass.');
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
  const replyState = {...state, requests: [], workspaces: [{id: 'w', name: 'repo', sessions: [
    {id: 'a', title: 'Fix it', unseen: true, reply, replySeq: 1, children: []},
  ]}]};
  const root = mount(createStore(replyState), <NotificationStack />);
  const preview = root.querySelector('.notification-card-text');

  expect(preview.classList.contains('notification-card-reply')).toBe(true);
  expect(preview.querySelector('strong').textContent).toBe('test paragraph');
  expect(preview.querySelector('em').textContent).toBe('markdown');
  expect(preview.querySelector('code').textContent).toBe('code');
  expect(preview.querySelector('a').getAttribute('href')).toBe('https://example.com/');
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

test('View Session shows the card\'s session', () => {
  const fetches = spyOn(globalThis, 'fetch').mockImplementation(async () => new Response(''));
  const store = createStore(state);
  const root = mount(store, <NotificationStack />);
  act(() => root.querySelectorAll('[title="View Session"]')[1].click());
  expect(store.get().selected).toBe('a');
  expect(root.querySelectorAll('.notification-card').length).toBe(1);
  fetches.mockRestore();
});

test('a reply card closes after 15 seconds without marking the reply seen', () => {
  jest.useFakeTimers();
  const fetches = spyOn(globalThis, 'fetch');
  const store = createStore(state);
  const root = mount(store, <NotificationStack />);
  act(() => jest.advanceTimersByTime(14999));
  expect(root.querySelectorAll('.notification-card').length).toBe(2);
  act(() => jest.advanceTimersByTime(1));
  expect(root.querySelectorAll('.notification-card').length).toBe(1);
  expect(store.get().dismissed).toEqual({'reply:a:4': true});
  expect(fetches).not.toHaveBeenCalled();
  fetches.mockRestore();
});

test('a refused action shows an error card, newest first, until it is closed', async () => {
  const fetches = spyOn(globalThis, 'fetch').mockImplementation(async () => new Response('session is busy', {status: 409}));
  const store = createStore({...state, workspaces: [], requests: []});
  const root = mount(store, <NotificationStack />);
  expect(await post(store, 'session', {action: 'stop', id: 'a'})).toBeNull();
  fetches.mockResolvedValue(new Response('no such session', {status: 404}));
  await post(store, 'session', {action: 'stop', id: 'b'});
  const texts = () => [...root.querySelectorAll('.notification-card')].map(card => card.textContent);
  expect(texts()).toEqual(['no such session', 'session is busy']);
  expect(root.querySelector('[title="View Session"]')).toBeNull();
  act(() => root.querySelector('[title="Close"]').click());
  expect(texts()).toEqual(['session is busy']);
  fetches.mockRestore();
});
