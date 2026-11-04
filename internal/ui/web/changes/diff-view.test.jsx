import {afterEach, expect, test} from 'bun:test';
import {mount, unmountAll} from '../../test-render.jsx';
import {emptySession} from '../sessions/selection.js';
import {createStore} from '../store.js';
import {DiffView} from './diff-view.jsx';

afterEach(unmountAll);

function view(diff) {
  const session = {...emptySession('s'), diffs: {'foo.py': diff}};
  return mount(createStore({session, workspaces: []}), <DiffView id="diff:foo.py" path="foo.py" />);
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
