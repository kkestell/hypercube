import {expect, test} from 'bun:test';
import {mentionChoices, prepareFiles} from './mention.js';

test('file results rank basename and contiguous matches before fuzzy matches and cap the list', () => {
  const files = prepareFiles(['foo/xabc.js', 'ab/c.js', 'a/b/c.js', 'abc.js', ...Array.from({length: 40}, (_, i) => `folder/${String(i).padStart(2, '0')}abc.js`)]);
  const choices = mentionChoices(files, 'abc', 4);
  expect(choices).toEqual(['abc.js', 'foo/xabc.js', 'folder/00abc.js', 'folder/01abc.js']);
  expect(mentionChoices(prepareFiles(['a/b/c.js', 'foo/abc/x', 'long/abc.js']), 'abc')).toEqual(['long/abc.js', 'foo/abc/x', 'a/b/c.js']);
});
