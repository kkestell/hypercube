import {afterEach, beforeEach, expect, spyOn, test} from 'bun:test';
import {mount, unmountAll} from '../../test-render.jsx';
import {createStore} from '../store.js';
import {receivedWorkspaces} from './selection.js';
import {SessionView} from './session-view.jsx';

let spies = [];
beforeEach(() => {
  spies.push(spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null));
});
afterEach(() => {
  for (const s of spies) s.mockRestore();
  spies = [];
  unmountAll();
});

const lead = {id: 'lead', name: 'lead', role: 'lead', activity: 'idle'};
const session = (id, extra = {}) => ({id, title: 'Session ' + id, running: true, unseen: false, agents: [lead], ...extra});
const store = (selected, sessions, visible = true) => createStore(receivedWorkspaces({
  workspaces: [], requests: [], selected, pending: null, unsent: null, visible, drafts: {}, errors: [], session: null,
}, [{id: 'w', name: 'repo', path: '/repo', sessions}]));

test('a selected session that is stopped shows the tesseract', () => {
  const s = store('a', [session('a', {running: false})]);
  expect(s.get().selected).toBe('a');
  const root = mount(s, <SessionView />);
  expect(root.querySelector('.tesseract')).not.toBeNull();
  expect(root.querySelector('.agent-pane')).toBeNull();
});

test('a running session shows its lead', () => {
  const root = mount(store('a', [session('a')]), <SessionView />);
  expect(root.querySelector('.tesseract')).toBeNull();
  expect(root.querySelector('.agent-pane')).not.toBeNull();
});

function seenPosts() {
  const fetches = spyOn(globalThis, 'fetch').mockImplementation(async () => new Response(''));
  spies.push(fetches);
  return () => fetches.mock.calls.map(([url]) => Object.fromEntries(new URL(url, 'http://x/').searchParams));
}

test('a visible page marks a shown session\'s unseen reply seen', () => {
  const posted = seenPosts();
  mount(store('a', [session('a', {unseen: true})]), <SessionView />);
  expect(posted()).toEqual([{action: 'seen', id: 'a'}]);
});

test('a hidden page marks the reply seen once it shows', () => {
  const posted = seenPosts();
  const s = store('a', [session('a', {unseen: true})], false);
  mount(s, <SessionView />);
  expect(posted()).toEqual([]);
  s.update(state => ({...state, visible: true}));
  expect(posted()).toEqual([{action: 'seen', id: 'a'}]);
});

test('a session with nothing unseen posts nothing', () => {
  const posted = seenPosts();
  mount(store('a', [session('a')]), <SessionView />);
  expect(posted()).toEqual([]);
});
