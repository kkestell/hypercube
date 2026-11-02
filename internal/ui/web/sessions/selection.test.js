import {expect, test} from 'bun:test';
import {unsentKey} from '../agent/drafts.js';
import {receivedTranscript} from '../transcript/transcripts.js';
import {openUnsent, pageTitle, receivedWorkspaces, selectSession} from './selection.js';

const session = (id, running, extra = {}) => ({id, title: 'Title ' + id, running, unseen: false, children: [], ...extra});
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

test('ended children attach and stay shown when they end', () => {
  const child = session('child', true, {parent: 'a'});
  let state = receivedWorkspaces(initial('child'), workspaces(session('a', true, {children: [child]})));
  expect(state.session.id).toBe('child');
  state = receivedTranscript(state, {session: 'child', reset: true, items: [{id: '1', seq: 1, kind: 'text'}]});
  state = receivedWorkspaces(state, workspaces(session('a', true, {children: [{...child, running: false, ended: true}]})));
  expect(state.selected).toBe('child');
  expect(state.session.transcript.order).toEqual(['1']);
  state = selectSession({...state, selected: null, session: null}, 'child');
  expect(state.session.id).toBe('child');
});

test('a child is deselected when its family stops', () => {
  const child = session('child', true, {parent: 'a'});
  let state = receivedWorkspaces(initial('child'), workspaces(session('a', true, {children: [child]})));
  state = receivedWorkspaces(state, workspaces(session('a', false, {children: [{...child, running: false}]})));
  expect(state.selected).toBeNull();
  expect(state.session).toBeNull();
});

test('pageTitle counts waiting sessions and names the session and workspace', () => {
  const state = receivedWorkspaces(initial('a'), workspaces(
    session('a', true, {attention: true, children: [session('x', true, {parent: 'a', attention: true})]}),
    session('b', false, {attention: true}),
  ));
  expect(pageTitle(state)).toBe('(2) Title a — repo — Hypercube');
  expect(pageTitle(initial())).toBe('Hypercube');
});
