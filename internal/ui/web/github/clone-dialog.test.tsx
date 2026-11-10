import {afterEach, expect, mock, test} from 'bun:test';
import {act} from 'react';
import {clickControl, mount, setValue, stubFetch, unmountAll} from '../../test-render.tsx';
import {testPullRequest, testState, testWorkspace} from '../../test-state.ts';
import {createStore} from '../store.ts';
import {CloneDialog} from './clone-dialog.tsx';

afterEach(() => { unmountAll(); mock.restore(); });

const github = {repo: 'o/a', url: '', account: '', branch: '', ahead: 0, behind: 0, pullRequests: [], updated: 1};
const setup = () => {
  const store = createStore(testState({workspaces: [testWorkspace({id: 'w', path: '/work/a', github})]}));
  let closed = 0;
  mount(store, <CloneDialog open pr={testPullRequest({repo: 'o/b'})} account="alice" onClose={() => { closed++; }} />);
  return {store, closed: () => closed};
};
const folder = () => {
  const label = [...document.querySelectorAll('label')].find(label => label.textContent === 'Folder')!;
  return document.getElementById(label.htmlFor) as HTMLInputElement;
};
const button = (label: string) => [...document.querySelectorAll('button')].find(b => b.textContent?.includes(label))!;
const flush = () => act(async () => { await new Promise(resolve => setTimeout(resolve, 0)); });

test('the folder is prefilled and the submit sends the repository, account and edited folder', async () => {
  const stub = stubFetch(() => new Response('new-ws'));
  const {store, closed} = setup();
  expect(folder().value).toBe('/work/b');
  act(() => { setValue(folder(), '/elsewhere/b'); folder().dispatchEvent(new Event('input', {bubbles: true})); });
  clickControl(button('Clone'));
  await flush();
  const [url, init] = stub.mock.calls[0];
  expect(init?.method).toBe('POST');
  const query = new URL(url, 'http://x').searchParams;
  expect(Object.fromEntries(query)).toEqual({action: 'clone', repo: 'o/b', account: 'alice', path: '/elsewhere/b'});
  expect(closed()).toBe(1);
  expect(store.get().pending).toBe('workspace:new-ws');
});

test('while cloning the field is disabled and the button says so; an error re-enables editing', async () => {
  let answer: (response: Response) => void = () => {};
  stubFetch(() => new Promise<Response>(resolve => { answer = resolve; }));
  const {closed} = setup();
  clickControl(button('Clone'));
  await flush();
  expect(folder().disabled).toBe(true);
  expect(button('Cloning…').disabled).toBe(true);
  expect(button('Cancel').disabled).toBe(false);
  await act(async () => { answer(new Response('destination path exists', {status: 409})); });
  expect(document.querySelector('[role="alert"]')?.textContent).toBe('destination path exists');
  expect(folder().disabled).toBe(false);
  expect(closed()).toBe(0);
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
