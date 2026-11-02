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

test('lead, workers, and files keep their order and independent column widths', () => {
  const s = store('a', [session('a', {agents: [lead, {id: 'worker', name: 'worker', role: 'work'}]})]);
  const root = mount(s, <SessionView />);
  const [outer, inner] = root.querySelectorAll('vscode-split-layout');
  const resize = (split, positionInPercentage) => act(() => split.dispatchEvent(new CustomEvent('vsc-split-layout-change', {detail: {positionInPercentage}})));
  expect(outer.classList.contains('split-collapsed')).toBe(false);
  expect(inner.classList.contains('split-collapsed')).toBe(true);
  expect([...root.querySelectorAll('.composer, .workers-panel, .editor-area')].map(element => element.className)).toEqual(['composer', 'workers-panel', 'editor-area']);
  s.update(state => ({...state, session: {...state.session, tabs: {...state.session.tabs, root: {id: 1, tabs: ['file:a.go'], active: 'file:a.go'}}}}));
  expect(inner.classList.contains('split-collapsed')).toBe(false);
  resize(outer, 40);
  resize(inner, 60);
  expect(outer.handlePosition).toBe('40%');
  expect(inner.handlePosition).toBe('60%');
  s.update(state => ({...state, session: {...state.session, tabs: {...state.session.tabs, root: null}}}));
  expect(outer.classList.contains('split-collapsed')).toBe(false);
  expect(inner.handlePosition).toBe('100%');
  expect(root.querySelector('.workers-panel')).not.toBeNull();
});

test('absent workers collapse their column and session switches reset worker expansion', () => {
  const worker = {id: 'worker', name: 'worker', role: 'work'};
  const s = store('a', [session('a', {agents: [lead, worker]}), session('b', {agents: [lead, worker]})]);
  const root = mount(s, <SessionView />);
  act(() => root.querySelector('.workers-panel-header').click());
  expect(root.querySelector('.workers-panel-header').getAttribute('aria-expanded')).toBe('true');
  s.update(state => ({...state, selected: 'b', session: {...state.session, id: 'b'}}));
  expect(root.querySelector('.workers-panel-header').getAttribute('aria-expanded')).toBe('false');
  s.update(state => receivedWorkspaces(state, [{id: 'w', sessions: [session('b')]}]));
  const [outer, inner] = root.querySelectorAll('vscode-split-layout');
  expect(outer.handlePosition).toBe('100%');
  expect(inner.handlePosition).toBe('0px');
});
