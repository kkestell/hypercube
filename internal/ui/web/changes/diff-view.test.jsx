import {afterEach, expect, test} from 'bun:test';
import {act} from 'react';
import {mount, unmountAll} from '../../test-render.jsx';
import {emptySession} from '../sessions/selection.js';
import {createStore} from '../store.js';
import {DiffView} from './diff-view.jsx';

afterEach(unmountAll);

function view(diff, path = 'foo.py') {
  const session = {...emptySession('s'), diffs: {[path]: diff}};
  return mount(createStore({session, workspaces: []}), <DiffView id={"diff:" + path} path={path} />);
}

test('a diff shows both line numbers, change marks, and changed line classes', () => {
  const root = view({text: '@@ -1,2 +1,2 @@\n context\n-old\n+new\n', message: null});
  const rows = [...root.querySelectorAll('.diff-view-line')];
  expect(rows.map(row => [...row.children].map(cell => cell.textContent))).toEqual([
    ['1', '1', '', 'context'], ['2', '', '−', 'old'], ['', '2', '+', 'new'],
  ]);
  expect(rows[1].classList.contains('diff-view-removed')).toBe(true);
  expect(rows[2].classList.contains('diff-view-added')).toBe(true);
});

test('an empty patch shows No changes and a refused file shows its message', () => {
  expect(view({text: '', message: null}).querySelector('.diff-view-message').textContent).toBe('No changes');
  expect(view({text: null, message: 'Too large'}).querySelector('.diff-view-message').textContent).toBe('Too large');
});


test.each([
  ['diff --git a/empty b/empty\nnew file mode 100644\nindex 0000000..e69de29\n', 'Empty file'],
  ['diff --git a/script b/script\nold mode 100644\nnew mode 100755\n', 'File mode changed from 100644 to 100755'],
])('metadata-only patches explain their change', (text, note) => {
  expect(view({text, message: null}).querySelector('.diff-view-message').textContent).toBe(note);
});

test('large line numbers are preserved on both sides', () => {
  const root = view({text: '@@ -9999 +100000 @@\n context\n', message: null});
  expect([...root.querySelectorAll('.diff-view-line td')].slice(0, 2).map(cell => cell.textContent)).toEqual(['9999', '100000']);
});

test('OCaml additions receive light and dark highlighting', async () => {
  const root = view({text: '@@ -0,0 +1 @@\n+let value = "hello"\n', message: null}, 'checked.ml');
  await act(async () => { await import('../files/highlight.js'); });
  const code = root.querySelector('code');
  expect(code.textContent).toBe('let value = "hello"');
  expect(code.innerHTML).toContain('--shiki-light:#D73A49');
  expect(code.innerHTML).toContain('--shiki-dark:#E6AC6F');
  expect(code.innerHTML).toContain('--shiki-dark:#98C379');
});
