import {afterEach, expect, spyOn, test} from 'bun:test';
import {emptySession} from '../sessions/selection.js';
import {createStore} from '../store.js';
import {readOpenFile} from './files.js';

let fetchMock;
afterEach(() => fetchMock?.mockRestore());

const unread = {text: null, message: null, etag: null, deleted: false};
const store = file => {
  const session = emptySession('s');
  return createStore({session: {...session, files: {...session.files, open: {'a.go': file}}}});
};
const read = async (s, response) => {
  fetchMock = spyOn(globalThis, 'fetch').mockImplementation(async () => response);
  await readOpenFile(s, 'a.go');
  fetchMock.mockRestore();
  return s.get().session.files.open['a.go'];
};
const text = body => new Response(body, {headers: {ETag: '"1"'}});

test('a read shows the text, and a later read replaces it', async () => {
  const s = store(unread);
  expect(await read(s, text('one'))).toEqual({text: 'one', message: null, etag: '"1"', deleted: false});
  expect((await read(s, text('two'))).text).toBe('two');
});

test('a deleted file keeps its text and loses its ETag', async () => {
  const s = store({text: 'one', message: null, etag: '"1"', deleted: false});
  expect(await read(s, new Response(null, {status: 404}))).toEqual({text: 'one', message: null, etag: null, deleted: true});
  const before = s.get();
  await read(s, new Response(null, {status: 404}));
  expect(s.get()).toBe(before);
});

test('a file that returns is no longer deleted', async () => {
  const s = store({text: 'one', message: null, etag: null, deleted: true});
  expect(await read(s, text('back'))).toEqual({text: 'back', message: null, etag: '"1"', deleted: false});
});

test('a failed read keeps what is shown, and shows the failure only when nothing is', async () => {
  const shown = store({text: 'one', message: null, etag: '"1"', deleted: false});
  const before = shown.get();
  await read(shown, new Response('permission denied', {status: 409}));
  expect(shown.get()).toBe(before);

  expect((await read(store(unread), new Response('permission denied', {status: 409}))).message).toBe('permission denied');
  fetchMock = spyOn(globalThis, 'fetch').mockRejectedValue(new TypeError('Load failed'));
  const unreached = store(unread);
  await readOpenFile(unreached, 'a.go');
  expect(unreached.get().session.files.open['a.go'].message).toBe('Load failed');
});

test('a large or binary file replaces the content with a message', async () => {
  const s = store({text: 'one', message: null, etag: '"1"', deleted: true});
  expect(await read(s, new Response(null, {status: 413}))).toEqual({
    text: null, message: 'The file is not displayed in the text editor because it is very large.', etag: null, deleted: false,
  });
  expect((await read(s, new Response(null, {status: 415}))).message).toContain('binary');
});

test('an unchanged file and a file that is no longer open are left alone', async () => {
  const s = store({text: 'one', message: null, etag: '"1"', deleted: false});
  const before = s.get();
  await read(s, new Response(null, {status: 304}));
  expect(s.get()).toBe(before);
  await readOpenFile(s, 'other.go');
  expect(s.get()).toBe(before);
});

test('a read preserves the file mode and requested highlights', async () => {
  const flags = {source: true, lines: [{start: 1, end: 1}], reveal: true};
  const s = store({...unread, ...flags});
  expect(await read(s, text('one'))).toMatchObject({...flags, text: 'one'});
  expect(await read(s, new Response(null, {status: 413}))).toMatchObject({...flags, text: null});
});
