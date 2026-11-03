import {afterEach, expect, spyOn, test} from 'bun:test';
import {act} from 'preact/test-utils';
import {mount, unmountAll} from '../../test-render.jsx';
import {createStore} from '../store.js';
import {MergeBar} from './merge-bar.jsx';

let fetches;
afterEach(() => {
  fetches?.mockRestore();
  fetches = null;
  unmountAll();
});

const merge = extra => ({branch: 'hypercube/session-a', target: 'main', unmerged: 0, committed: false, uncommitted: 0, ...extra});
const render = (m, running = true) => {
  const session = {id: 'a', running, worktree: true, merge: m, children: []};
  const store = createStore({workspaces: [{id: 'w', path: '/repo', sessions: [session]}], errors: []});
  return mount(store, <MergeBar session={session} />);
};
const text = root => root.querySelector('.merge-bar-text').textContent;
const button = root => root.querySelector('.merge-bar-merge');

test.each([
  [merge(), 'hypercube/session-a · No commits yet'],
  [merge({uncommitted: 2}), 'hypercube/session-a · No commits yet · 2 uncommitted files'],
  [merge({unmerged: 3, committed: true}), 'hypercube/session-a · 3 commits not merged into main'],
  [merge({unmerged: 1, committed: true, uncommitted: 1}), 'hypercube/session-a · 1 commit not merged into main · 1 uncommitted file'],
  [merge({committed: true}), 'hypercube/session-a · Merged into main'],
  [merge({committed: true, uncommitted: 4}), 'hypercube/session-a · Merged into main · 4 uncommitted files'],
  [merge({error: 'no such directory'}), 'hypercube/session-a · Merge status unavailable: no such directory'],
])('the merge bar reads %o as %s', (m, want) => {
  expect(text(render(m))).toBe(want);
});

test('Merge shows only with unmerged commits and no error', () => {
  for (const m of [merge(), merge({committed: true}), merge({uncommitted: 1}), merge({error: 'x'})]) {
    expect(button(render(m))).toBeNull();
    unmountAll();
  }
  expect(button(render(merge({unmerged: 1, committed: true}), false)).hasAttribute('disabled')).toBe(true);
});

test('Merge asks the agent to merge into the target in the checkout', async () => {
  fetches = spyOn(globalThis, 'fetch').mockImplementation(async () => new Response(''));
  const root = render(merge({unmerged: 2, committed: true}));
  expect(button(root).hasAttribute('disabled')).toBe(false);
  await act(() => button(root).click());
  const [url, init] = fetches.mock.calls[0];
  expect(Object.fromEntries(new URL(url, 'http://x/').searchParams)).toEqual({action: 'input', id: 'a'});
  expect(JSON.parse(init.body)).toEqual({
    text: 'Commit any remaining work, then merge your branch into main in the checkout at /repo, resolving any conflicts, and report the result.',
    images: [],
  });
});
