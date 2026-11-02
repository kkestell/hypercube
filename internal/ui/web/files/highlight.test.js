import {expect, test} from 'bun:test';
import {highlight} from './highlight.js';

test('files highlight in their language and unknown files as escaped plain text', () => {
  expect(highlight('package main\n', 'go')).toEqual(['<span style="color:#569CD6;">package</span><span style="color:#D4D4D4;"> </span><span style="color:#4EC9B0;">main</span>', '']);
  expect(highlight('<b>', '')).toEqual(['&#60;b&#62;']);
  expect(highlight('ls', 'shellscript')[0]).toContain('color:');
});
