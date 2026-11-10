import {afterEach, expect, mock, test} from 'bun:test';
import {act} from 'react';
import {element, mount, setValue, stubFetch, unmountAll} from '../../test-render.tsx';
import {testState} from '../../test-state.ts';
import {emptyEditor} from '../sessions/selection.ts';
import {createStore} from '../store.ts';
import {SearchPanel} from './search-panel.tsx';

afterEach(() => { unmountAll(); mock.restore(); });

test('search replaces old results, cancels an unfinished query, and announces errors', async () => {
  const requests: Array<{signal: AbortSignal; answer: (response: Response) => void}> = [];
  stubFetch((_url, init) => new Promise<Response>(answer => requests.push({signal: init!.signal!, answer})));
  const root = mount(createStore(testState({editor: emptyEditor({workspace: 'w'})})), <SearchPanel />);
  const input = element(root, '[aria-label="Search files contents"]', HTMLInputElement);
  const search = (query: string) => act(() => {
    setValue(input, query);
    input.dispatchEvent(new Event('input', {bubbles: true}));
  });
  search('first');
  expect(root.textContent).toContain('Searching files.');
  search('second');
  expect(requests[0].signal.aborted).toBe(true);
  await act(async () => {
    requests[1].answer(Response.json([{path: 'main.go', lines: [{line: 7, parts: ['a ', 'second', ' result']}]}]));
    requests[0].answer(Response.json([{path: 'old.go', lines: []}]));
  });
  expect(root.textContent).toContain('main.go');
  expect(root.textContent).not.toContain('old.go');
  search('third');
  expect(root.textContent).not.toContain('main.go');
  await act(async () => requests[2].answer(new Response('permission denied', {status: 403})));
  expect(element(root, '[role="alert"]', HTMLElement).textContent).toBe('Could not search files: permission denied');
  search('');
  expect(root.querySelector('[role="alert"]')).toBeNull();
});
