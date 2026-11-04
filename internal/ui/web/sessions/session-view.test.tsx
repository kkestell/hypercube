import {afterEach, beforeEach, expect, mock, spyOn, test} from 'bun:test';
import {find, mount, stubFetch, unmountAll} from '../../test-render.tsx';
import {testSession, testState, testWorkspace} from '../../test-state.ts';
import type {Profile} from '../agent/types.ts';
import {createStore} from '../store.ts';
import type {Session} from './types.ts';
import {openUnsent, receivedWorkspaces, selectSession, unsentProfile} from './selection.ts';
import {SessionView} from './session-view.tsx';

beforeEach(() => {
  spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null);
});
afterEach(() => {
  mock.restore();
  unmountAll();
});

const session = (id: string, extra: Partial<Session> = {}) => testSession({id, title: 'Session ' + id, running: true, ...extra});
const workspaces = (sessions: Session[]) => [testWorkspace({id: 'w', name: 'repo', path: '/repo', sessions})];
const store = (selected: string | null, sessions: Session[], visible = true) =>
  createStore(receivedWorkspaces(testState({selected, visible}), workspaces(sessions)));

test('a selected stopped session attaches and shows its transcript', () => {
  const s = store('a', [session('a', {running: false})]);
  expect(s.get().selected).toBe('a');
  const root = mount(s, <SessionView />);
  expect(root.querySelector('.tesseract')).toBeNull();
  expect(root.querySelector('.agent-pane')).not.toBeNull();
});

test('a running session shows its agent in a tab named for it', () => {
  const root = mount(store('a', [session('a')]), <SessionView />);
  expect(root.querySelector('.tesseract')).toBeNull();
  expect(root.querySelector('.agent-pane')).not.toBeNull();
  expect(find(root, '[data-tab="agent"] .editor-tabs-label').textContent).toBe('Session a');
});

test('with no session shown, the tab strip is empty but for the layout toggles', () => {
  const root = mount(store(null, []), <SessionView />);
  expect(root.querySelector('.tesseract')).toBeNull();
  expect(root.querySelector('.editor-tabs-bar')).not.toBeNull();
  expect(root.querySelectorAll('.layout-toggles button')).toHaveLength(3);
  expect(root.querySelector('[role="tab"]')).toBeNull();
});

function seenPosts() {
  const fetches = stubFetch(() => new Response(''));
  return () => fetches.mock.calls.map(([url]) => Object.fromEntries(new URL(String(url), 'http://x/').searchParams));
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

test('an unsent session shows the tesseract in place of its transcript', () => {
  const lead: Profile = {name: 'lead', description: '', harness: 'claude', model: 'haiku', effort: 'low'};
  const s = store(null, []);
  s.update(state => unsentProfile(openUnsent(state, 'w'), lead));
  const root = mount(s, <SessionView />);
  expect(root.querySelector('.agent-pane .tesseract')).not.toBeNull();
  expect(root.querySelector('.transcript')).toBeNull();
  expect(root.querySelector('.composer textarea[aria-label="Message"]')).not.toBeNull();
  expect(find(root, '[data-tab="agent"] .editor-tabs-label').textContent).toBe('New Session');
  s.update(state => selectSession(receivedWorkspaces(state, workspaces([session('a')])), 'a'));
  expect(root.querySelector('.tesseract')).toBeNull();
  expect(root.querySelector('.transcript')).not.toBeNull();
});
