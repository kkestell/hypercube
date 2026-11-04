import {expect, test} from 'bun:test';
import {highlight} from './highlight.js';

test('files highlight in their language and unknown files as escaped plain text', () => {
  const lines = highlight('package main\n', 'go');
  expect(lines[0]).toContain('color:');
  expect(lines[0].replace(/<[^>]*>/g, '')).toBe('package main');
  expect(lines[1]).toBe('');
  expect(highlight('<b>', '')).toEqual(['&#60;b&#62;']);
  expect(highlight('ls', 'shellscript')[0]).toContain('color:');
});

test('OCaml uses both light and dark keyword and string colors', () => {
  const [line] = highlight('let value = "hello"', 'ocaml');
  expect(line).toContain('--shiki-light:#D73A49');
  expect(line).toContain('--shiki-dark:#E6AC6F');
  expect(line).toContain('--shiki-dark:#98C379');
  expect(line.replace(/<[^>]*>/g, '')).toBe('let value = &#34;hello&#34;');
});
