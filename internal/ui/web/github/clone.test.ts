import {expect, test} from 'bun:test';
import {testWorkspace} from '../../test-state.ts';
import type {Workspace} from '../sessions/types.ts';
import {suggestedFolder, tracking} from './clone.ts';

const github = (repo: string): Workspace['github'] => ({repo, url: '', account: '', branch: '', ahead: 0, behind: 0, pullRequests: [], updated: 0});
const workspaces = [
  testWorkspace({id: 'plain', path: '/home/plain'}),
  testWorkspace({id: 'a', path: '/work/strib/a', github: github('MinneapolisStarTribune/a')}),
  testWorkspace({id: 'b', path: '/me/b', github: github('me/b')}),
];

test('a repository is tracked regardless of case, and workspaces without GitHub track nothing', () => {
  expect(tracking(workspaces, 'minneapolisstartribune/A')?.id).toBe('a');
  expect(tracking(workspaces, 'me/c')).toBeUndefined();
  expect(tracking([workspaces[0]], 'plain/plain')).toBeUndefined();
});

test('the suggested folder sits beside a workspace of the same owner', () => {
  expect(suggestedFolder(workspaces, 'MinneapolisStarTribune/b')).toBe('/work/strib/b');
  expect(suggestedFolder(workspaces, 'minneapolisstartribune/b')).toBe('/work/strib/b');
});

test('without a workspace of the same owner the suggestion sits beside the first workspace', () => {
  expect(suggestedFolder(workspaces, 'other/x')).toBe('/home/x');
});
