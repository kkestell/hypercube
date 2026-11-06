import {afterEach, expect, mock, test} from 'bun:test';
import {act} from 'react';
import {element, mount, stubFetch, unmountAll} from '../../test-render.tsx';
import {testSession, testState, testWorkspace} from '../../test-state.ts';
import {receivedWorkspaces, selectSession} from '../sessions/selection.ts';
import {createStore} from '../store.ts';
import {FilesPanel} from './files-panel.tsx';

afterEach(() => { unmountAll(); mock.restore(); });

test('a restored tree requests expanded folders immediately and first displays them together', async () => {
  const store = createStore(receivedWorkspaces(testState({selected: 'a'}), [testWorkspace({
    id: 'w', sessions: [testSession({id: 'a'}), testSession({id: 'b'})],
  })]));
  store.update(state => ({...state, session: {...state.session!, files: {
    ...state.session!.files, expanded: {src: true, 'src/nested': true},
  }}}));
  store.update(state => selectSession(state, 'b'));
  store.update(state => selectSession(state, 'a'));
  const pending = new Map<string, (response: Response) => void>();
  stubFetch(url => {
    const {pathname, searchParams} = new URL(url, 'http://x/');
    if (pathname !== '/files') return Response.json([]);
    return new Promise<Response>(resolve => pending.set(searchParams.get('path')!, resolve));
  });
  const root = mount(store, <FilesPanel />);
  expect([...pending.keys()]).toEqual(['', 'src', 'src/nested']);
  expect(root.querySelector('[role=treeitem]')).toBeNull();
  await act(async () => pending.get('')!(Response.json([{name: 'src', dir: true}])));
  expect(root.querySelector('[role=treeitem]')).toBeNull();
  await act(async () => pending.get('src')!(Response.json([{name: 'nested', dir: true}])));
  expect(root.querySelector('[role=treeitem]')).toBeNull();
  await act(async () => pending.get('src/nested')!(Response.json([{name: 'file.go', dir: false}])));
  expect(element(root, '[data-path="src"]', HTMLElement).getAttribute('aria-expanded')).toBe('true');
  expect(element(root, '[data-path="src/nested"]', HTMLElement).getAttribute('aria-expanded')).toBe('true');
  expect(element(root, '[data-path="src/nested/file.go"]', HTMLElement).textContent).toBe('file.go');
});
