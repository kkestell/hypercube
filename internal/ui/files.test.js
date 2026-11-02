import {afterEach, expect, spyOn, test} from 'bun:test';
import {fileIcon, files, language} from './static/files.js';
import theme from './static/vs-seti-icon-theme.json';
import {highlight} from './viewer.js';

let fetchMock;
afterEach(() => fetchMock?.mockRestore());

test('files.list sends the session and relative directory', async () => {
  const entries = [{name: 'a.go', dir: false}];
  fetchMock = spyOn(globalThis, 'fetch').mockResolvedValue(Response.json(entries));
  expect(await files.list('session one', 'sub/folder')).toEqual(entries);
  const [url, options] = fetchMock.mock.calls[0];
  const query = new URL(url, 'http://localhost').searchParams;
  expect(query.get('session')).toBe('session one');
  expect(query.get('path')).toBe('sub/folder');
  expect(options.cache).toBe('no-store');
});

test('files.list preserves the status of a failed listing', async () => {
  fetchMock = spyOn(globalThis, 'fetch').mockResolvedValue(new Response('missing', {status: 404}));
  await expect(files.list('s', 'missing')).rejects.toMatchObject({message: 'missing', status: 404});
});

test('files.read returns text and ETag and sends the previous ETag', async () => {
  fetchMock = spyOn(globalThis, 'fetch').mockResolvedValue(new Response('package main\n', {headers: {ETag: '"13-123"'}}));
  expect(await files.read('s', 'a.go', '"12-122"')).toEqual({text: 'package main\n', etag: '"13-123"'});
  const [url, options] = fetchMock.mock.calls[0];
  const query = new URL(url, 'http://localhost').searchParams;
  expect(query.get('session')).toBe('s');
  expect(query.get('path')).toBe('a.go');
  expect(options.headers).toEqual({'If-None-Match': '"12-122"'});
});

test('files.read maps unchanged, deleted, large and binary responses', async () => {
  fetchMock = spyOn(globalThis, 'fetch');
  for (const [status, result] of [
    [304, {unchanged: true}],
    [404, {deleted: true}],
    [413, {message: 'The file is not displayed in the text editor because it is very large.'}],
    [415, {message: 'The file is not displayed in the text editor because it is either binary or uses an unsupported text encoding.'}],
  ]) {
    fetchMock.mockResolvedValue(new Response(null, {status}));
    expect(await files.read('s', 'a.go')).toEqual(result);
  }
});

test('files.read reports other server failures with their error text', async () => {
  fetchMock = spyOn(globalThis, 'fetch').mockResolvedValue(new Response('permission denied', {status: 409}));
  expect(await files.read('s', 'a.go')).toEqual({message: 'permission denied'});
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
