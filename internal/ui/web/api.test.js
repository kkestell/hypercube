import {afterEach, expect, spyOn, test} from 'bun:test';
import {listFiles, readFile} from './api.js';

let fetchMock;
afterEach(() => fetchMock?.mockRestore());

const query = call => new URL(call[0], 'http://localhost/').searchParams;

test('listFiles sends the session and relative directory', async () => {
  const entries = [{name: 'a.go', dir: false}];
  fetchMock = spyOn(globalThis, 'fetch').mockResolvedValue(Response.json(entries));
  expect(await listFiles('session one', 'sub/folder')).toEqual(entries);
  expect(query(fetchMock.mock.calls[0]).get('session')).toBe('session one');
  expect(query(fetchMock.mock.calls[0]).get('path')).toBe('sub/folder');
  expect(fetchMock.mock.calls[0][1].cache).toBe('no-store');
});

test('listFiles keeps the status of a failed listing', async () => {
  fetchMock = spyOn(globalThis, 'fetch').mockResolvedValue(new Response('missing', {status: 404}));
  await expect(listFiles('s', 'missing')).rejects.toMatchObject({message: 'missing', status: 404});
});

test('readFile returns text and ETag and sends the previous ETag', async () => {
  fetchMock = spyOn(globalThis, 'fetch').mockResolvedValue(new Response('package main\n', {headers: {ETag: '"13-123"'}}));
  expect(await readFile('s', 'a.go', '"12-122"')).toEqual({text: 'package main\n', etag: '"13-123"'});
  expect(query(fetchMock.mock.calls[0]).get('path')).toBe('a.go');
  expect(fetchMock.mock.calls[0][1].headers).toEqual({'If-None-Match': '"12-122"'});
});

test('readFile maps unchanged, deleted, large, and binary files', async () => {
  fetchMock = spyOn(globalThis, 'fetch');
  for (const [status, result] of [
    [304, {unchanged: true}],
    [404, {deleted: true}],
    [413, {message: 'The file is not displayed in the text editor because it is very large.'}],
    [415, {message: 'The file is not displayed in the text editor because it is either binary or uses an unsupported text encoding.'}],
  ]) {
    fetchMock.mockResolvedValue(new Response(null, {status}));
    expect(await readFile('s', 'a.go')).toEqual(result);
  }
});

test('readFile reports other failures with their text', async () => {
  fetchMock = spyOn(globalThis, 'fetch').mockResolvedValue(new Response('permission denied', {status: 409}));
  expect(await readFile('s', 'a.go')).toEqual({error: 'permission denied'});
});
