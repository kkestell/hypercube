import {expect, test} from 'bun:test';
import {findMatches} from './find-matches.js';

test('finds case-insensitive text across inline markup without crossing blocks', () => {
  const root = document.createElement('div');
  document.body.append(root);
  root.innerHTML = '<div style="display:block">A <strong style="display:inline">bold</strong> word</div><div style="display:block">WORD</div>';
  expect(findMatches(root, 'a bold').map(range => range.toString())).toEqual(['A bold']);
  expect(findMatches(root, 'word').map(range => range.toString())).toEqual(['word', 'WORD']);
  expect(findMatches(root, 'wordword')).toEqual([]);
  expect(findMatches(root, '')).toEqual([]);
  root.remove();
});
