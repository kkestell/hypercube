import {expect, test} from 'bun:test';
import {transcriptMatches} from './transcript-matches.js';

test('finds case-insensitive text across inline markup without crossing blocks', () => {
  const root = document.createElement('div');
  document.body.append(root);
  root.innerHTML = '<div style="display:block">A <strong style="display:inline">bold</strong> word</div><div style="display:block">WORD</div>';
  expect(transcriptMatches(root, 'a bold').map(range => range.toString())).toEqual(['A bold']);
  expect(transcriptMatches(root, 'word').map(range => range.toString())).toEqual(['word', 'WORD']);
  expect(transcriptMatches(root, 'wordword')).toEqual([]);
  expect(transcriptMatches(root, '')).toEqual([]);
  root.remove();
});
