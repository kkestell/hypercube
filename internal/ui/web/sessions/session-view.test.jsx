import {afterEach, beforeEach, expect, spyOn, test} from 'bun:test';
import {act} from 'preact/test-utils';
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

const session = (id, extra = {}) => ({id, title: 'Session ' + id, running: true, unseen: false, children: [], activity: 'idle', ...extra});
const store = (selected, sessions, visible = true) => createStore(receivedWorkspaces({
  workspaces: [], requests: [], selected, pending: null, unsent: null, visible, drafts: {}, errors: [], session: null,
}, [{id: 'w', name: 'repo', path: '/repo', sessions}]));

test('a selected stopped session attaches and shows its transcript', () => {
  const s = store('a', [session('a', {running: false})]);
  expect(s.get().selected).toBe('a');
  const root = mount(s, <SessionView />);
  expect(root.querySelector('.tesseract')).toBeNull();
  expect(root.querySelector('.agent-pane')).not.toBeNull();
});

test('a running session shows its agent pane', () => {
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

test('the session and files share one split', () => {
  const s = store('a', [session('a', {children: [session('child', {parent: 'a'})]})]);
  const root = mount(s, <SessionView />);
  const splits = root.querySelectorAll('vscode-split-layout');
  expect(splits.length).toBe(1);
  expect(root.querySelectorAll('.agent-pane').length).toBe(1);
  const split = splits[0];
  expect(split.classList.contains('split-collapsed')).toBe(true);
  s.update(state => ({...state, session: {...state.session, tabs: {...state.session.tabs, root: {id: 1, tabs: ['file:a.go'], active: 'file:a.go'}}}}));
  expect(split.classList.contains('split-collapsed')).toBe(false);
  act(() => split.dispatchEvent(new CustomEvent('vsc-split-layout-change', {detail: {positionInPercentage: 40}})));
  expect(split.handlePosition).toBe('40%');
});
