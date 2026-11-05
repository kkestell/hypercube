import {afterEach, expect, mock, spyOn, test} from 'bun:test';
import {stubFetch} from '../../test-render.tsx';
import {emptySession} from '../sessions/selection.ts';
import {createStore, type Store} from '../store.ts';
import {testState, unread} from '../../test-state.ts';
import {readOpenFile} from './files.ts';
import type {OpenFile} from './types.ts';

afterEach(() => mock.restore());

const store = (file: OpenFile) => {
  const session = emptySession('s');
  return createStore(testState({session: {...session, files: {...session.files, open: {'a.go': file}}}}));
};
const opened = (s: Store) => s.get().session?.files.open['a.go'];
const read = async (s: Store, response: Response) => {
  stubFetch(() => response);
  await readOpenFile(s, 'a.go');
  mock.restore();
  return opened(s);
};
const text = (body: string) => new Response(body, {headers: {ETag: '"1"'}});

test('a read shows the text, and a later read replaces it', async () => {
  const s = store(unread);
  expect(await read(s, text('one'))).toEqual({...unread, text: 'one', etag: '"1"'});
  expect((await read(s, text('two')))?.text).toBe('two');
});

test('a deleted file keeps its text and loses its ETag', async () => {
  const s = store({...unread, text: 'one', etag: '"1"'});
  expect(await read(s, new Response(null, {status: 404}))).toEqual({...unread, text: 'one', deleted: true});
  const before = s.get();
  await read(s, new Response(null, {status: 404}));
  expect(s.get()).toBe(before);
});

test('a file that returns is no longer deleted', async () => {
  const s = store({...unread, text: 'one', deleted: true});
  expect(await read(s, text('back'))).toEqual({...unread, text: 'back', etag: '"1"'});
});

test('a failed read keeps what is shown, and shows the failure only when nothing is', async () => {
  const shown = store({...unread, text: 'one', etag: '"1"'});
  const before = shown.get();
  await read(shown, new Response('permission denied', {status: 409}));
  expect(shown.get()).toBe(before);

  expect((await read(store(unread), new Response('permission denied', {status: 409})))?.message).toBe('permission denied');
  spyOn(globalThis, 'fetch').mockRejectedValue(new TypeError('Load failed'));
  const unreached = store(unread);
  await readOpenFile(unreached, 'a.go');
  expect(opened(unreached)?.message).toBe('Load failed');
});

test('a large or binary file replaces the content with a message', async () => {
  const s = store({...unread, text: 'one', etag: '"1"', deleted: true});
  expect(await read(s, new Response(null, {status: 413}))).toEqual({
    ...unread, message: 'The file is not displayed in the text editor because it is very large.',
  });
  expect((await read(s, new Response(null, {status: 415})))?.message).toContain('binary');
});

test('an unchanged file and a file that is no longer open are left alone', async () => {
  const s = store({...unread, text: 'one', etag: '"1"'});
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

test('browser previews refresh, survive deletion and failed reads, and yield to text or unsupported files', async () => {
  const s = store(unread);
  const preview = {mime: 'image/png', url: '/preview/s/repo/picture.png?v=1'};
  await read(s, Response.json({text: null, preview}, {headers: {ETag: '"1"'}}));
  expect(opened(s)).toMatchObject({text: null, preview, etag: '"1"'});
  const before = s.get();
  await read(s, new Response(null, {status: 304}));
  expect(s.get()).toBe(before);
  await read(s, new Response('denied', {status: 409}));
  expect(s.get()).toBe(before);
  await read(s, new Response(null, {status: 404}));
  expect(opened(s)).toMatchObject({preview, deleted: true});
  const changed = {...preview, url: '/preview/s/repo/picture.png?v=2'};
  await read(s, Response.json({text: null, preview: changed}, {headers: {ETag: '"2"'}}));
  expect(opened(s)).toMatchObject({preview: changed, deleted: false, etag: '"2"'});
  await read(s, text('new text'));
  expect(opened(s)?.preview).toBeUndefined();
  await read(s, Response.json({text: null, preview}));
  await read(s, new Response(null, {status: 415}));
  expect(opened(s)?.preview).toBeUndefined();
  expect(opened(s)?.message).toContain('binary');
});
