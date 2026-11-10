// Failing comparisons of page nodes report quickly, and other comparisons
// keep Bun's own matchers.
import {expect, test} from 'bun:test';
import {mount} from './test-render.tsx';
import {testSession, testState, testWorkspace} from './test-state.ts';
import {receivedWorkspaces} from './web/sessions/selection.ts';
import {createStore} from './web/store.ts';
import {App} from './web/app/app.tsx';

test('dom failure is fast', () => {
  const store = createStore(receivedWorkspaces(testState({selected: 'a'}), [testWorkspace({id: 'w', name: 'repo', path: '/repo', sessions: [testSession({id: 'a'})]})]));
  const root = mount(store, <App store={store} />);
  const t = performance.now();
  expect(() => expect([root.querySelector('[aria-label=Chat]')]).toEqual([])).toThrow('aria-label="Chat"');
  expect([root.querySelector('[aria-label=Chat]')]).toEqual([root.querySelector('[aria-label=Chat]')]);
  expect(performance.now() - t).toBeLessThan(100);
});
test('plain values keep Bun diff and matchers', () => {
  expect({a: 1, b: undefined}).toEqual({a: 1});
  expect(() => expect({a: 1, b: undefined}).toStrictEqual({a: 1})).toThrow();
  expect({a: 1, b: 'x'}).toEqual({a: 1, b: expect.any(String)});
  expect([1, 2]).not.toEqual([1, 3]);
  expect(() => expect([1, 2]).toEqual([1, 3])).toThrow('Expected');
});
