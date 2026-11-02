import {expect, test} from 'bun:test';
import {fileIcon, files, language} from './static/files.js';
import theme from './static/vs-seti-icon-theme.json';
import {highlight} from './viewer.js';

test('the stub lists one directory at a time', async () => {
  const names = entries => entries.map(e => `${e.name}${e.dir ? '/' : ''}`).sort();
  expect(names(await files.list('cmd'))).toEqual(['hypercube/']);
  expect(names(await files.list(''))).toContain('internal/');
  expect(names(await files.list(''))).toContain('go.mod');
  expect(await files.read('go.mod')).toStartWith('module ');
});

test('Seti icons match a file name, then its extensions, then its language', () => {
  const glyph = name => fileIcon(theme, name).match(/color:(#\w+)">&#x(\w+);/).slice(1);
  const icon = id => [theme.iconDefinitions[id].fontColor, theme.iconDefinitions[id].fontCharacter.slice(1)];
  expect(glyph('README.md')).toEqual(icon(theme.fileNames['readme.md']));
  expect(glyph('main.go')).toEqual(icon(theme.languageIds.go));
  expect(glyph('Makefile')).toEqual(icon(theme.languageIds.makefile));
  expect(glyph('x.d.ts')).toEqual(icon(theme.fileExtensions['d.ts'] ?? theme.languageIds.typescript));
  expect(glyph('notes.unknown')).toEqual(icon(theme.file));
});

test('files highlight in their language and unknown files as plain text', () => {
  expect(language('main.go')).toBe('go');
  expect(language('Dockerfile')).toBe('dockerfile');
  expect(language('notes.unknown')).toBe('');
  expect(highlight('package main\n', 'go')).toEqual(['<span style="color:#569CD6;">package</span><span style="color:#D4D4D4;"> </span><span style="color:#4EC9B0;">main</span>', '']);
  expect(highlight('<b>', '')).toEqual(['&#60;b&#62;']);
  expect(highlight('ls', 'sh')[0]).toContain('color:');
});
