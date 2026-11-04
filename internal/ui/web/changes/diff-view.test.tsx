import {afterEach, expect, test} from 'bun:test';
import {act} from 'react';
import {element, find, mount, setValue, unmountAll} from '../../test-render.tsx';
import {testState} from '../../test-state.ts';
import {emptySession} from '../sessions/selection.ts';
import {createStore} from '../store.ts';
import {DiffView} from './diff-view.tsx';
import type {OpenDiff} from './types.ts';

afterEach(unmountAll);


const session = (path: string, diff: OpenDiff) => ({...emptySession('s'), diffs: {[path]: diff}});

function view(diff: OpenDiff, path = 'foo.py') {
  return mount(createStore(testState({session: session(path, diff)})), <DiffView id={'diff:' + path} path={path} />);
}

const message = (diff: OpenDiff) => view(diff).querySelector('.diff-view-message')?.textContent;

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
  expect(message({text: '', message: null})).toBe('No changes');
  expect(message({text: null, message: 'Too large'})).toBe('Too large');
});

test.each([
  ['diff --git a/empty b/empty\nnew file mode 100644\nindex 0000000..e69de29\n', 'Empty file'],
  ['diff --git a/script b/script\nold mode 100644\nnew mode 100755\n', 'File mode changed from 100644 to 100755'],
])('metadata-only patches explain their change', (text, note) => {
  expect(message({text, message: null})).toBe(note);
});

test('large line numbers are preserved on both sides', () => {
  const root = view({text: '@@ -9999 +100000 @@\n context\n', message: null});
  expect([...root.querySelectorAll('.diff-view-line td')].slice(0, 2).map(cell => cell.textContent)).toEqual(['9999', '100000']);
});

test('OCaml additions receive light and dark highlighting', async () => {
  const root = view({text: '@@ -0,0 +1 @@\n+let value = "hello"\n', message: null}, 'checked.ml');
  await act(async () => { await import('../files/highlight.ts'); });
  const code = find(root, 'code');
  expect(code.textContent).toBe('let value = "hello"');
  expect(code.innerHTML).toContain('--shiki-light:#D73A49');
  expect(code.innerHTML).toContain('--shiki-dark:#E6AC6F');
  expect(code.innerHTML).toContain('--shiki-dark:#98C379');
});

test('find and change navigation operate on the focused diff', () => {
  const path = 'foo.py';
  const diff = {text: '@@ -1,4 +1,4 @@\n-old\n+one\n context\n-before\n+one again\n', message: null};
  const root = mount(createStore(testState({session: session(path, diff)})), <div className="editor-pane"><DiffView id="diff:foo.py" path={path} /></div>);
  const scrolled: Element[] = [];
  const view = find(root, '.diff-view');
  const changes = [...root.querySelectorAll<HTMLElement>('[data-change]')];
  view.getBoundingClientRect = () => new DOMRect(0, 0, 0, 0);
  changes.forEach((row, index) => {
    row.getBoundingClientRect = () => new DOMRect(0, 100 + index * 100 - view.scrollTop, 0, 0);
    row.scrollIntoView = () => { scrolled.push(row); view.scrollTop = 100 + index * 100; };
  });
  view.focus();
  const nextChange = (shiftKey = false) => act(() => { view.dispatchEvent(new KeyboardEvent('keydown', {key: 'F5', altKey: true, shiftKey, bubbles: true, cancelable: true})); });
  nextChange();
  nextChange();
  view.scrollTop = 50;
  nextChange();
  nextChange(true);
  expect(scrolled).toEqual([changes[0], changes[1], changes[0], changes[1]]);
  act(() => { view.dispatchEvent(new KeyboardEvent('keydown', {key: 'f', metaKey: true, bubbles: true, cancelable: true})); });
  const input = element(root, '[role=search] input', HTMLInputElement);
  act(() => { setValue(input, 'one'); input.dispatchEvent(new Event('input', {bubbles: true})); });
  expect(find(root, '[aria-live=polite]').textContent).toBe('1 of 2');
  act(() => { input.dispatchEvent(new KeyboardEvent('keydown', {key: 'Enter', bubbles: true})); });
  expect(find(root, '[aria-live=polite]').textContent).toBe('2 of 2');
  act(() => { input.dispatchEvent(new KeyboardEvent('keydown', {key: 'Escape', bubbles: true})); });
  expect(root.querySelector('[role=search]')).toBeNull();
  expect(document.activeElement).toBe(view);
});

test('the shared wrap preference changes diff line wrapping', () => {
  const path = 'foo.py';
  const store = createStore(testState({session: session(path, {text: '@@ -1 +1 @@\n-old\n+new\n', message: null})}));
  const root = mount(store, <DiffView id="diff:foo.py" path={path} />);
  expect(find(root, '.diff-view-line td:last-child').className).toContain('whitespace-pre ');
  act(() => store.update(state => ({...state, workbench: {...state.workbench, wrap: true}})));
  expect(find(root, '.diff-view-line td:last-child').className).toContain('whitespace-pre-wrap');
});
