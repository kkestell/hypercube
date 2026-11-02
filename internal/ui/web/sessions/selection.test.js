import {expect, test} from 'bun:test';
import {unsentKey} from '../agent/drafts.js';
import {openWorkerTab, rememberScroll} from '../editor/editor.js';
import {receivedTranscript} from '../transcript/transcripts.js';
import {openUnsent, pageTitle, receivedWorkspaces, selectSession} from './selection.js';

const agent = (id, extra = {}) => ({id, name: id, role: 'work', activity: 'idle', ...extra});
const session = (id, running, agents = [agent(id + '-lead', {role: 'lead'})]) => ({id, title: 'Title ' + id, running, unseen: false, agents});
const workspaces = (...sessions) => [{id: 'w', name: 'repo', path: '/repo', sessions}];
const initial = (selected = '') => ({workspaces: [], requests: [], selected, pending: null, unsent: null, drafts: {}, session: null});

test('a page without a hash selects a running session', () => {
  const state = receivedWorkspaces(initial(), workspaces(session('a', false), session('b', true)));
  expect(state.selected).toBe('b');
  expect(state.session.id).toBe('b');
});

test('a pending selection waits for the state that lists it', () => {
  let state = receivedWorkspaces(initial(), workspaces(session('a', true)));
  state = selectSession(state, 'new-one');
  expect(state.pending).toBe('new-one');
  expect(state.selected).toBe('a');
  state = receivedWorkspaces(state, workspaces(session('a', true), session('new-one', true)));
  expect(state.selected).toBe('new-one');
  expect(state.pending).toBeNull();
  expect(state.session.id).toBe('new-one');
});

test('a session that stops while shown is deselected', () => {
  let state = receivedWorkspaces(initial('a'), workspaces(session('a', true), session('b', true)));
  state = receivedWorkspaces(state, workspaces(session('a', false), session('b', true)));
  expect(state.selected).toBeNull();
  expect(state.session).toBeNull();
});

test('a removed selection moves to a running session', () => {
  let state = receivedWorkspaces(initial('a'), workspaces(session('a', true), session('b', true)));
  state = receivedWorkspaces(state, workspaces(session('b', true)));
  expect(state.selected).toBe('b');
  state = receivedWorkspaces(state, workspaces(session('c', false)));
  expect(state.selected).toBe('');
});

test('the unsent session is discarded with its workspace', () => {
  let state = receivedWorkspaces(initial(), workspaces(session('a', true)));
  state = openUnsent(state, 'w', {name: 'claude-lead', harness: 'claude', model: 'opus', effort: 'high'});
  state = {...state, drafts: {[unsentKey]: {text: 'hi', images: [], reading: 0}}};
  expect(state.selected).toBeNull();
  expect(state.session).toBeNull();
  expect(state.unsent.harness).toBe('claude');
  state = receivedWorkspaces(state, []);
  expect(state.unsent).toBeNull();
  expect(state.drafts).toEqual({});
});

test('attaching to another session replaces the session state', () => {
  let state = receivedWorkspaces(initial('a'), workspaces(session('a', true), session('b', true)));
  const attached = state.session;
  state = receivedWorkspaces(state, workspaces(session('a', true), session('b', true)));
  expect(state.session).toBe(attached);
  state = selectSession(state, 'b');
  expect(state.session).toEqual({...attached, id: 'b'});
  expect(state.session).not.toBe(attached);
});

test('a departing agent loses its tab, transcript, and scroll', () => {
  const lead = agent('lead', {role: 'lead'}), worker = agent('worker');
  let state = receivedWorkspaces(initial('a'), workspaces(session('a', true, [lead, worker])));
  state = receivedTranscript(state, {session: 'a', agent: 'worker', reset: true, items: [{id: '1', seq: 1, kind: 'text'}]});
  state = rememberScroll(openWorkerTab(state, 'worker'), 'a', 'worker', {top: 1, left: 0, following: false});
  state = receivedWorkspaces(state, workspaces(session('a', true, [lead])));
  expect(state.session.transcripts).toEqual({});
  expect(state.session.scroll).toEqual({});
  expect(state.session.tabs.root).toBeNull();
});

test('pageTitle counts waiting agents and names the session and workspace', () => {
  const state = receivedWorkspaces(initial('a'), workspaces(
    session('a', true, [agent('l', {role: 'lead', attention: true}), agent('x', {attention: true})]),
    session('b', false, [agent('m', {role: 'lead', attention: true})]),
  ));
  expect(pageTitle(state)).toBe('(2) Title a — repo — Hypercube');
  expect(pageTitle(initial())).toBe('Hypercube');
});
