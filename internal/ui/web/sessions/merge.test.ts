import {expect, test} from 'bun:test';
import {branchState, branchSummary} from './merge.ts';
import type {Merge} from './types.ts';

const merge = (extra: Partial<Merge> = {}): Merge => ({branch: 'b', target: 'main', unmerged: 0, committed: true, uncommitted: 0, ...extra});

test('the branch state puts attention first: error, uncommitted files, unmerged commits, merged, empty', () => {
  expect(branchState(merge({error: 'x', uncommitted: 2, unmerged: 1}))).toBe('error');
  expect(branchState(merge({uncommitted: 2, unmerged: 1}))).toBe('uncommitted');
  expect(branchState(merge({unmerged: 1}))).toBe('unmerged');
  expect(branchState(merge())).toBe('merged');
  expect(branchState(merge({committed: false}))).toBe('empty');
});

test('the branch summary leads with uncommitted files', () => {
  expect(branchSummary(merge({uncommitted: 3, unmerged: 2}))).toBe('3 uncommitted files, 2 commits not merged into main');
  expect(branchSummary(merge({uncommitted: 1}))).toBe('1 uncommitted file, merged into main');
  expect(branchSummary(merge({unmerged: 1}))).toBe('1 commit not merged into main');
});
