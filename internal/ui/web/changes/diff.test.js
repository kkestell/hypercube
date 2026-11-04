import {expect, test} from 'bun:test';
import {diffRows} from './diff.js';

test('unified rows skip metadata and number each side independently', () => {
  const patch = 'diff --git a/a b/a\n--- a/a\n+++ b/a\n@@ -1,3 +1,3 @@\n same\n-old\n+new\n end\n\\ No newline at end of file\n';
  expect(diffRows(patch)).toEqual([
    {kind: 'context', old: 1, new: 1, text: 'same'},
    {kind: 'removed', old: 2, new: null, text: 'old'},
    {kind: 'added', old: null, new: 2, text: 'new'},
    {kind: 'context', old: 3, new: 3, text: 'end'},
  ]);
  expect(diffRows('')).toEqual([]);
});

test('new files, empty context lines, and multiple hunk starts retain their numbers', () => {
  expect(diffRows('@@ -0,0 +1,2 @@\n+one\n+\n@@ -5 +8 @@\n \n')).toEqual([
    {kind: 'added', old: null, new: 1, text: 'one'},
    {kind: 'added', old: null, new: 2, text: ''},
    {kind: 'context', old: 5, new: 8, text: ''},
  ]);
});
