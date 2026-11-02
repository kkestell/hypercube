import {afterEach, expect, jest, spyOn, test} from 'bun:test';
import {act} from 'preact/test-utils';
import {mount, unmountAll} from '../../test-render.jsx';
import {createStore} from '../store.js';
import {NotificationStack} from './notification-stack.jsx';

afterEach(() => {
  jest.useRealTimers();
  unmountAll();
});

const workspaces = [{id: 'w', name: 'repo', sessions: [
  {id: 'a', title: 'Fix it', unseen: true, reply: 'Done.\nAll tests pass.', replySeq: 4, agents: [{id: 'a-lead', name: 'lead', role: 'lead'}]},
  {id: 'b', title: 'Ask', unseen: false, agents: [{id: 'b-lead', name: 'lead', role: 'lead'}, {id: 'b-w', name: 'auditor', role: 'work'}]},
]}];
const requests = [{agent: 'b-w', item: {id: 'q', kind: 'attention', text: 'Allow?', choices: ['Yes']}}];
const state = {workspaces, requests, selected: '', pending: null, unsent: null, drafts: {}, dismissed: {}, session: null};

test('cards show their source and request answers', () => {
  const root = mount(createStore(state), <NotificationStack />);
  const [request, reply] = root.querySelectorAll('.notification-card');
  expect(request.querySelector('.notification-card-source').textContent).toBe('repo — Ask — auditor');
  expect(request.querySelector('vscode-button').textContent).toBe('Yes');
  expect(reply.querySelector('.notification-card-source').textContent).toBe('repo — Fix it');
  expect(reply.querySelector('.notification-card-text').textContent).toBe('Done.\nAll tests pass.');
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
