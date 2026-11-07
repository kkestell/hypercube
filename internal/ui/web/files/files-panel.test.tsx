import {afterEach, expect, mock, test} from 'bun:test';
import {act} from 'react';
import {element, mount, stubFetch, unmountAll} from '../../test-render.tsx';
import {testSession, testState, testWorkspace} from '../../test-state.ts';
import {receivedWorkspaces, selectSession} from '../sessions/selection.ts';
import {createStore} from '../store.ts';
import {FilesPanel} from './files-panel.tsx';
import {createFilePoller} from './file-polling.ts';
import {emptySession} from '../sessions/selection.ts';

afterEach(() => { unmountAll(); mock.restore(); });

test('a restored tree shows expanded folders from one response', async () => {
  const store = createStore(receivedWorkspaces(testState({selected: 'a'}), [testWorkspace({
    id: 'w', sessions: [testSession({id: 'a'}), testSession({id: 'b'})],
  })]));
  store.update(state => ({...state, session: {...state.session!, files: {
    ...state.session!.files, expanded: {src: true, 'src/nested': true},
  }}}));
  store.update(state => selectSession(state, 'b'));
  store.update(state => selectSession(state, 'a'));
  let release = (_response: Response) => {};
  const fetches = stubFetch(url => {
    const {pathname} = new URL(url, 'http://x/');
    if (pathname !== '/files/tree') return Response.json([]);
    return new Promise<Response>(resolve => { release = resolve; });
  });
  const root = mount(store, <FilesPanel />);
  expect(JSON.parse(fetches.mock.calls[0]?.[1]?.body as string)).toEqual(['', 'src', 'src/nested']);
  expect(root.querySelector('[role=treeitem]')).toBeNull();
  await act(async () => release(Response.json({
    '': [{name: 'src', dir: true}],
    src: [{name: 'nested', dir: true}],
    'src/nested': [{name: 'file.go', dir: false}],
  })));
  expect(element(root, '[data-path="src"]', HTMLElement).getAttribute('aria-expanded')).toBe('true');
  expect(element(root, '[data-path="src/nested"]', HTMLElement).getAttribute('aria-expanded')).toBe('true');
  expect(element(root, '[data-path="src/nested/file.go"]', HTMLElement).textContent).toBe('file.go');
});

test('a visited session shows its cached tree before refresh completes', async () => {
  const store = createStore(receivedWorkspaces(testState({selected: 'a'}), [testWorkspace({
    id: 'w', sessions: [testSession({id: 'a'}), testSession({id: 'b'})],
  })]));
  store.update(state => ({...state, session: {...state.session!, files: {
    ...state.session!.files, listings: {'': [{name: 'src', dir: true}], src: [{name: 'file.go', dir: false}]},
    expanded: {src: true},
  }}}));
  store.update(state => selectSession(state, 'b'));
  store.update(state => selectSession(state, 'a'));
  stubFetch(() => new Promise<Response>(() => {}));
  const root = mount(store, <FilesPanel />);
  expect(element(root, '[data-path="src/file.go"]', HTMLElement).textContent).toBe('file.go');
});

test('a denied root listing is visible, preserves cached entries, and clears on the next successful poll', async () => {
  const session = emptySession('s');
  session.files.listings = {'': [{name: 'cached.go', dir: false}]};
  const store = createStore(testState({session}));
  let denied = true;
  stubFetch(url => {
    if (new URL(url, 'http://x/').pathname !== '/files/tree') return Response.json(null);
    return denied ? new Response('permission denied', {status: 403}) : Response.json({'': [{name: 'fresh.go', dir: false}]});
  });
  let root!: HTMLDivElement;
  await act(async () => { root = mount(store, <FilesPanel />); });
  expect(root.textContent).toContain('permission denied');
  expect(root.textContent).toContain('cached.go');
  expect(store.get().errors).toEqual([]);
  denied = false;
  await act(async () => { await createFilePoller(store)(); });
  expect(root.textContent).not.toContain('permission denied');
  expect(root.textContent).toContain('fresh.go');
});
