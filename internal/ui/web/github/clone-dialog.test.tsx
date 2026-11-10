import {afterEach, expect, mock, test} from 'bun:test';
import {act} from 'react';
import {clickControl, mount, setValue, stubFetch, testBridge, unmountAll} from '../../test-render.tsx';
import {testPullRequest, testState, testWorkspace} from '../../test-state.ts';
import {createStore} from '../store.ts';
import {CloneDialog} from './clone-dialog.tsx';

afterEach(() => { unmountAll(); mock.restore(); delete globalThis.hypercube; });

const github = {repo: 'o/a', url: '', account: '', branch: '', ahead: 0, behind: 0, pullRequests: [], updated: 1};
const setup = () => {
  const store = createStore(testState({workspaces: [testWorkspace({id: 'w', path: '/work/a', github})]}));
  let closed = 0;
  const added: string[] = [];
  mount(store, <CloneDialog open pr={testPullRequest({repo: 'o/b'})} account="alice" onAdded={id => { added.push(id); }} onClose={() => { closed++; }} />);
  return {closed: () => closed, added};
};
const folder = () => {
  const label = [...document.querySelectorAll('label')].find(label => label.textContent === 'Folder')!;
  return document.getElementById(label.htmlFor) as HTMLInputElement;
};
const button = (label: string) => [...document.querySelectorAll('button')].find(b => b.textContent?.includes(label))!;
const flush = () => act(async () => { await new Promise(resolve => setTimeout(resolve, 0)); });

test('the folder is prefilled and the submit sends the repository, account and edited folder', async () => {
  const stub = stubFetch(() => new Response('new-ws'));
  const {closed, added} = setup();
  expect(folder().value).toBe('/work/b');
  act(() => { setValue(folder(), '/elsewhere/b'); folder().dispatchEvent(new Event('input', {bubbles: true})); });
  clickControl(button('Clone'));
  await flush();
  const [url, init] = stub.mock.calls[0];
  expect(init?.method).toBe('POST');
  const query = new URL(url, 'http://x').searchParams;
  expect(Object.fromEntries(query)).toEqual({action: 'clone', repo: 'o/b', account: 'alice', path: '/elsewhere/b'});
  expect(closed()).toBe(1);
  expect(added).toEqual(['new-ws']);
});

test('while cloning the field is disabled and the button says so; an error re-enables editing', async () => {
  let answer: (response: Response) => void = () => {};
  stubFetch(() => new Promise<Response>(resolve => { answer = resolve; }));
  const {closed, added} = setup();
  clickControl(button('Clone'));
  await flush();
  expect(folder().disabled).toBe(true);
  expect(button('Cloning…').disabled).toBe(true);
  expect(button('Cancel').disabled).toBe(false);
  await act(async () => { answer(new Response('destination path exists', {status: 409})); });
  expect(document.querySelector('[role="alert"]')?.textContent).toBe('destination path exists');
  expect(folder().disabled).toBe(false);
  expect(closed()).toBe(0);
  expect(added).toEqual([]);
});

test('closing the dialog aborts the request and shows no error', async () => {
  let signal: AbortSignal | null | undefined;
  stubFetch((_url, init) => new Promise<Response>((_resolve, reject) => {
    signal = init?.signal;
    signal?.addEventListener('abort', () => reject(new DOMException('aborted', 'AbortError')));
  }));
  const {closed} = setup();
  clickControl(button('Clone'));
  await flush();
  clickControl(button('Cancel'));
  await flush();
  expect(signal?.aborted).toBe(true);
  expect(document.querySelector('[role="alert"]')).toBeNull();
  expect(closed()).toBe(1);
});

const existing = 'Use Existing Folder…';

test('Use Existing Folder adds the chosen folder for the repository', async () => {
  globalThis.hypercube = testBridge({chooseFolder: async () => '/src/b'});
  const stub = stubFetch(() => new Response('ws-b'));
  const {closed, added} = setup();
  clickControl(button(existing));
  await flush();
  expect(Object.fromEntries(new URL(stub.mock.calls[0][0], 'http://x').searchParams)).toEqual({action: 'new', path: '/src/b', repo: 'o/b'});
  expect(stub.mock.calls[0][1]?.method).toBe('POST');
  expect(added).toEqual(['ws-b']);
  expect(closed()).toBe(1);
});

test('a folder that is not a checkout of the repository is refused in the dialog', async () => {
  globalThis.hypercube = testBridge({chooseFolder: async () => '/src/other'});
  stubFetch(() => new Response('/src/other is a checkout of o/other, not o/b', {status: 409}));
  const {closed, added} = setup();
  clickControl(button(existing));
  await flush();
  expect(document.querySelector('[role="alert"]')?.textContent).toBe('/src/other is a checkout of o/other, not o/b');
  expect(added).toEqual([]);
  expect(closed()).toBe(0);
});

test('choosing no folder adds nothing', async () => {
  globalThis.hypercube = testBridge();
  const stub = stubFetch(() => new Response('unexpected'));
  const {closed} = setup();
  clickControl(button(existing));
  await flush();
  expect(stub).not.toHaveBeenCalled();
  expect(closed()).toBe(0);
});

test('without the desktop app there is no existing-folder choice', () => {
  setup();
  expect(button(existing)).toBeUndefined();
});
