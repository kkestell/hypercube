import {expect, test} from 'bun:test';
import {testConfig, testSession, testState, testWorkspace} from '../../test-state.ts';
import {receivedConfig} from '../app/config.ts';
import {draftOf, typed, unsentKey as workspaceDraftKey} from '../agent/drafts.ts';
import {receivedTranscript} from '../transcript/transcripts.ts';
import type {Session} from './types.ts';
import {emptySession, openUnsent, pageTitle, receivedWorkspaces, selectSession, unsentSettingsChosen, unsentWorktree} from './selection.ts';

const unsentKey = workspaceDraftKey('w');

const session = (id: string, ended = false, extra: Partial<Session> = {}) => testSession({id, title: 'Title ' + id, ended, ...extra});
const workspaces = (...sessions: Session[]) => [testWorkspace({id: 'w', name: 'repo', path: '/repo', sessions})];
const initial = (selected = '') => testState({selected});

test('new-session drafts and choices survive navigation across sessions and workspaces', () => {
  let state = receivedWorkspaces(initial('a'), [...workspaces(session('a')), testWorkspace({id: 'other'})]);
  state = typed(unsentWorktree(unsentSettingsChosen(openUnsent(state, 'w'), {harness: 'claude', model: 'opus', effort: 'high'}), true), unsentKey, 'unfinished');
  const identity = state.unsent?.identity;
  state = selectSession(state, 'a');
  expect(state.unsent).toBeNull();
  expect(state.session?.id).toBe('a');
  expect(draftOf(state, unsentKey).text).toBe('unfinished');
  state = typed(openUnsent(state, 'other'), workspaceDraftKey('other'), 'another workspace');
  state = openUnsent(state, 'w');
  expect(state.unsent).toMatchObject({identity, settings: {harness: 'claude', model: 'opus', effort: 'high'}, worktree: true});
  expect(draftOf(state, unsentKey).text).toBe('unfinished');
  expect(draftOf(state, workspaceDraftKey('other')).text).toBe('another workspace');
  expect(state.unsentDrafts[unsentKey]).toBeUndefined();
  expect(openUnsent(state, 'w')).toBe(state);
  state = receivedWorkspaces(state, workspaces(session('a')));
  expect(state.unsentDrafts[workspaceDraftKey('other')]).toBeUndefined();
  expect(draftOf(state, workspaceDraftKey('other')).text).toBe('');
});

test('a page without a hash selects a session that has not ended', () => {
  const state = receivedWorkspaces(initial(), workspaces(session('a', true), session('b')));
  expect(state.selected).toBe('b');
  expect(state.session?.id).toBe('b');
});

test('a pending selection waits for the state that lists it', () => {
  let state = receivedWorkspaces(initial(), workspaces(session('a')));
  state = selectSession(state, 'new-one');
  expect(state.pending).toBe('new-one');
  expect(state.selected).toBe('a');
  state = receivedWorkspaces(state, workspaces(session('a'), session('new-one')));
  expect(state.selected).toBe('new-one');
  expect(state.pending).toBeNull();
  expect(state.session?.id).toBe('new-one');
});

test('a session archived while shown is deselected', () => {
  let state = receivedWorkspaces(initial('a'), workspaces(session('a'), session('b')));
  state = receivedWorkspaces(state, workspaces(session('a', true), session('b')));
  expect(state.selected).toBeNull();
  expect(state.session).toBeNull();
});

test('a removed selection moves to a session that has not ended', () => {
  let state = receivedWorkspaces(initial('a'), workspaces(session('a'), session('b')));
  state = receivedWorkspaces(state, workspaces(session('b')));
  expect(state.selected).toBe('b');
  state = receivedWorkspaces(state, workspaces(session('c', true)));
  expect(state.selected).toBe('');
});

test('the unsent session is discarded with its workspace', () => {
  let state = receivedWorkspaces(initial(), workspaces(session('a')));
  state = openUnsent(state, 'w');
  // A new draft starts with the settings of the workspace's newest session.
  expect(state.unsent?.settings).toEqual({harness: '', model: '', effort: ''});
  state = unsentSettingsChosen(state, {harness: 'claude', model: 'opus', effort: 'high'});
  state = {...state, drafts: {[unsentKey]: {text: 'hi', images: [], reading: 0}}};
  expect(state.selected).toBeNull();
  expect(state.session).toBeNull();
  expect(state.unsent?.settings).toEqual({harness: 'claude', model: 'opus', effort: 'high'});
  state = receivedWorkspaces(state, []);
  expect(state.unsent).toBeNull();
  expect(state.drafts).toEqual({});
});

test('attaching to another session replaces the session state', () => {
  let state = receivedWorkspaces(initial('a'), workspaces(session('a'), session('b')));
  const attached = state.session;
  state = receivedWorkspaces(state, workspaces(session('a'), session('b')));
  expect(state.session).toBe(attached);
  state = selectSession(state, 'b');
  expect(state.session).toEqual(emptySession('b'));
  expect(state.session).not.toBe(attached);
});

test('ended children attach and stay shown when they end', () => {
  const child = session('child', false, {parent: 'a'});
  let state = receivedWorkspaces(initial('child'), workspaces(session('a', false, {children: [child]})));
  expect(state.session?.id).toBe('child');
  state = receivedTranscript(state, {type: 'transcript', session: 'child', reset: true, items: [{id: '1', seq: 1, kind: 'text'}]});
  state = receivedWorkspaces(state, workspaces(session('a', false, {children: [{...child, ended: true}]})));
  expect(state.selected).toBe('child');
  expect(state.session?.transcript.order).toEqual(['1']);
  state = selectSession({...state, selected: null, session: null}, 'child');
  expect(state.session?.id).toBe('child');
});

test('a child is deselected when its family is archived', () => {
  const child = session('child', false, {parent: 'a'});
  let state = receivedWorkspaces(initial('child'), workspaces(session('a', false, {children: [child]})));
  state = receivedWorkspaces(state, workspaces(session('a', true, {children: [{...child, ended: true}]})));
  expect(state.selected).toBeNull();
  expect(state.session).toBeNull();
});

test('pageTitle counts waiting sessions and names the session and workspace', () => {
  const state = receivedWorkspaces(initial('a'), workspaces(
    session('a', false, {attention: true, children: [session('x', false, {parent: 'a', attention: true})]}),
    session('b'),
  ));
  expect(pageTitle(state)).toBe('(2) Title a — repo — Hypercube');
  expect(pageTitle(initial())).toBe('Hypercube');
});


test('waiting counts include saved offers in parked sessions once per session', () => {
  const state = receivedWorkspaces(initial('a'), workspaces(session('a'), session('b', false, {attention: true})));
  const requests = ['a', 'a', 'b'].map((id, i) => ({session: id, item: {id: String(i), kind: 'attention' as const}}));
  expect(pageTitle({...state, requests})).toBe('(2) Title a — repo — Hypercube');
});

test("a draft's settings are set once and then follow neither new sessions nor a new default", () => {
  const opus = {harness: 'claude', model: 'opus', effort: 'high'};
  let state = receivedWorkspaces(initial(), workspaces());
  state = typed(openUnsent(state, 'w'), unsentKey, 'unfinished');
  // Nothing supplies settings yet; the config, when it arrives, does once.
  expect(state.unsent?.settings).toBeNull();
  state = receivedConfig(state, testConfig({default_selection: opus}));
  expect(state.unsent?.settings).toEqual(opus);
  state = receivedConfig(state, testConfig({default_selection: {harness: 'codex', model: 'sol', effort: 'low'}}));
  state = receivedWorkspaces(state, workspaces(session('n', false, {harness: 'pi', model: 'p/x', effort: 'max'})));
  expect(state.unsent?.settings).toEqual(opus);
  expect(draftOf(state, unsentKey).text).toBe('unfinished');
  // Parked and reopened, the draft keeps them.
  state = openUnsent(selectSession(state, 'n'), 'w');
  expect(state.unsent?.settings).toEqual(opus);
});
