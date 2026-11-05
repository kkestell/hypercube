import {afterEach, expect, mock, spyOn, test} from 'bun:test';
import {testState} from '../test-state.ts';
import {fetchConfig, listFiles, post, readFile} from './api.ts';
import type {FileRead} from './files/types.ts';
import {createStore, type Store} from './store.ts';

afterEach(() => mock.restore());

const errors = (store: Store) => store.get().errors.map(error => error.message);

// requested returns the URL and options of the first fetch.
function requested(fetchMock: {mock: {calls: Parameters<typeof fetch>[]}}) {
  const [input, init] = fetchMock.mock.calls[0];
  return {url: new URL(String(input), 'http://localhost/'), init};
}

test('listFiles sends the session and relative directory', async () => {
  const entries = [{name: 'a.go', dir: false}];
  const fetchMock = spyOn(globalThis, 'fetch').mockResolvedValue(Response.json(entries));
  expect(await listFiles('session one', 'sub/folder')).toEqual(entries);
  const {url, init} = requested(fetchMock);
  expect(url.searchParams.get('session')).toBe('session one');
  expect(url.searchParams.get('path')).toBe('sub/folder');
  expect(init?.cache).toBe('no-store');
});

test('listFiles keeps the status of a failed listing', async () => {
  spyOn(globalThis, 'fetch').mockResolvedValue(new Response('missing', {status: 404}));
  await expect(listFiles('s', 'missing')).rejects.toMatchObject({message: 'missing', status: 404});
});

test('readFile returns text and ETag and sends the previous ETag', async () => {
  const fetchMock = spyOn(globalThis, 'fetch').mockResolvedValue(new Response('package main\n', {headers: {ETag: '"13-123"'}}));
  expect(await readFile('s', 'a.go', '"12-122"')).toEqual({text: 'package main\n', etag: '"13-123"'});
  const {url, init} = requested(fetchMock);
  expect(url.searchParams.get('path')).toBe('a.go');
  expect(init?.headers).toEqual({'If-None-Match': '"12-122"'});
});

test('readFile maps unchanged, deleted, large, and binary files', async () => {
  const fetchMock = spyOn(globalThis, 'fetch');
  const cases: [number, FileRead][] = [
    [304, {unchanged: true}],
    [404, {deleted: true}],
    [413, {message: 'The file is not displayed in the text editor because it is very large.'}],
    [415, {message: 'The file is not displayed in the text editor because it is either binary or uses an unsupported text encoding.'}],
  ];
  for (const [status, result] of cases) {
    fetchMock.mockResolvedValue(new Response(null, {status}));
    expect(await readFile('s', 'a.go')).toEqual(result);
  }
});

test('readFile reports other failures with their text', async () => {
  spyOn(globalThis, 'fetch').mockResolvedValue(new Response('permission denied', {status: 409}));
  expect(await readFile('s', 'a.go')).toEqual({error: 'permission denied'});
});

test('readFile reads browser preview metadata without downloading media bytes', async () => {
  const preview = {mime: 'image/png', url: '/preview/s/repo/picture.png?v=1'};
  spyOn(globalThis, 'fetch').mockResolvedValue(Response.json({text: null, preview}, {headers: {ETag: '"1"'}}));
  expect(await readFile('s', 'picture.png')).toEqual({text: null, preview, etag: '"1"'});
});

test('post returns the response text', async () => {
  const fetchMock = spyOn(globalThis, 'fetch').mockResolvedValue(new Response('ok'));
  expect(await post(createStore(testState()), 'session', {action: 'input'}, 'body')).toBe('ok');
  expect(requested(fetchMock).init).toEqual({method: 'POST', body: 'body'});
});

test('post shows a refusal and returns null', async () => {
  spyOn(globalThis, 'fetch').mockResolvedValue(new Response('busy', {status: 409}));
  const store = createStore(testState());
  expect(await post(store, 'session', {})).toBeNull();
  expect(errors(store)).toEqual(['busy']);
});

test('post shows a network failure and returns null', async () => {
  spyOn(globalThis, 'fetch').mockRejectedValue(new TypeError('Load failed'));
  const store = createStore(testState());
  expect(await post(store, 'session', {})).toBeNull();
  expect(errors(store)).toEqual(['Load failed']);
});

test('fetchConfig requests every configured profile', async () => {
  const profiles = ['one', 'two'].map(name => ({name, description: '', harness: 'claude', model: '', effort: ''}));
  const fetchMock = spyOn(globalThis, 'fetch').mockResolvedValue(Response.json({default_profile: "two", profiles}));
  expect(await fetchConfig(createStore(testState()))).toEqual({default_profile: "two", profiles});
  expect(fetchMock.mock.calls[0][0]).toBe('config');
});

test('fetchConfig shows malformed profiles and returns null', async () => {
  spyOn(globalThis, 'fetch').mockResolvedValue(new Response('not json'));
  const store = createStore(testState());
  expect(await fetchConfig(store)).toBeNull();
  expect(errors(store)).toHaveLength(1);
});

for (const status of [200, 409]) {
  test(`post shows a failed response body (${status}) and returns null`, async () => {
    const body = new ReadableStream({
      start(controller) { controller.error(new TypeError('Connection lost')); },
    });
    spyOn(globalThis, 'fetch').mockResolvedValue(new Response(body, {status}));
    const store = createStore(testState());
    expect(await post(store, 'session', {})).toBeNull();
    expect(errors(store)).toHaveLength(1);
    expect(errors(store)[0]).toContain('Connection lost');
  });
}
