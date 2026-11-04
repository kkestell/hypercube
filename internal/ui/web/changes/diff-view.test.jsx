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

test('find and change navigation operate on the focused diff', () => {
  const path = 'foo.py';
  const session = {...emptySession('s'), diffs: {[path]: {text: '@@ -1,4 +1,4 @@\n-old\n+one\n context\n-before\n+one again\n', message: null}}};
  const root = mount(createStore({session, workspaces: [], workbench: {wrap: false}}), <div className="editor-pane"><DiffView id="diff:foo.py" path={path} /></div>);
  const scrolled = [];
  for (const row of root.querySelectorAll('[data-change]')) row.scrollIntoView = () => scrolled.push(row);
  const view = root.querySelector('.diff-view');
  view.focus();
  act(() => view.dispatchEvent(new KeyboardEvent('keydown', {key: 'F5', altKey: true, bubbles: true, cancelable: true})));
  act(() => view.dispatchEvent(new KeyboardEvent('keydown', {key: 'F5', altKey: true, bubbles: true, cancelable: true})));
  act(() => view.dispatchEvent(new KeyboardEvent('keydown', {key: 'F5', altKey: true, shiftKey: true, bubbles: true, cancelable: true})));
  expect(scrolled).toEqual([root.querySelectorAll('[data-change]')[0], root.querySelectorAll('[data-change]')[1], root.querySelectorAll('[data-change]')[0]]);
  act(() => view.dispatchEvent(new KeyboardEvent('keydown', {key: 'f', metaKey: true, bubbles: true, cancelable: true})));
  const input = root.querySelector('[role=search] input');
  act(() => { Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(input, 'one'); input.dispatchEvent(new Event('input', {bubbles: true})); });
  expect(root.querySelector('[aria-live=polite]').textContent).toBe('1 of 2');
  act(() => input.dispatchEvent(new KeyboardEvent('keydown', {key: 'Enter', bubbles: true})));
  expect(root.querySelector('[aria-live=polite]').textContent).toBe('2 of 2');
  act(() => input.dispatchEvent(new KeyboardEvent('keydown', {key: 'Escape', bubbles: true})));
  expect(root.querySelector('[role=search]')).toBeNull();
  expect(document.activeElement).toBe(view);
});

test('the shared wrap preference changes diff line wrapping', () => {
  const path = 'foo.py';
  const session = {...emptySession('s'), diffs: {[path]: {text: '@@ -1 +1 @@\n-old\n+new\n', message: null}}};
  const store = createStore({session, workspaces: [], workbench: {wrap: false}});
  const root = mount(store, <DiffView id="diff:foo.py" path={path} />);
  expect(root.querySelector('.diff-view-line td:last-child').className).toContain('whitespace-pre ');
  act(() => store.update(state => ({...state, workbench: {...state.workbench, wrap: true}})));
  expect(root.querySelector('.diff-view-line td:last-child').className).toContain('whitespace-pre-wrap');
});
