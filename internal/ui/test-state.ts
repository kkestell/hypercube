// Complete states, workspaces, and sessions for tests, which override only
// what a test is about.
import type {Config} from './web/app/types.ts';
import type {OpenFile} from './web/files/types.ts';
import type {Session, Workspace} from './web/sessions/types.ts';
import {initialState, type State} from './web/state.ts';

export const testState = (overrides: Partial<State> = {}): State => ({...initialState('', true), ...overrides});

export const testWorkspace = (overrides: Partial<Workspace> & {id: string}): Workspace => ({
  path: '/' + overrides.id, name: overrides.id, noGit: false, sessions: [], ...overrides,
});

export const testSession = (overrides: Partial<Session> & {id: string}): Session => ({
  parent: '', title: overrides.id, ready: false, ended: false, unseen: false,
  harness: '', model: '', effort: '', dir: '', activity: 'idle', attention: false, usage: {},
  worktree: false, children: [], ...overrides,
});

// testConfig has a default selection and the default preferences.
export const testConfig = (overrides: Partial<Config> = {}): Config => ({
  default_selection: {harness: 'claude', model: 'opus', effort: 'high'}, child_model_guidance: '',
  default_worktree: false, theme: 'system', terminal_font_size: 13,
  notifications: {requests: true, replies: true, desktop: true, sound: true},
  ...overrides,
});

// unread is a file open in a tab that has not been read.
export const unread: OpenFile = {text: null, message: null, etag: null, deleted: false, source: false, lines: [], reveal: false};
