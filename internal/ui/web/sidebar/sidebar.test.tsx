import {afterEach, expect, mock, test} from 'bun:test';
import {act} from 'react';
import {clickControl, element, find, findAll, mount, setValue, stubFetch, unmountAll} from '../../test-render.tsx';
import {testSession, testState, testWorkspace} from '../../test-state.ts';
import type {Session} from '../sessions/types.ts';
import {ConfirmDialog} from '../shared/confirm-dialog.tsx';
import {createStore} from '../store.ts';
import {Sidebar} from './sidebar.tsx';

afterEach(() => {
  mock.restore();
  unmountAll();
});

const session = (id: string, extra: Partial<Session> = {}) => testSession({id, title: 'Session ' + id, running: true, ...extra});
function setup(sessions: Session[]) {
  const requests: URL[] = [];
  stubFetch(url => {
    requests.push(new URL(url, 'http://x/'));
    return new Response('');
  });
  const store = createStore(testState({selected: null, workspaces: [testWorkspace({id: 'w', name: 'repo', path: '/repo', sessions})]}));
  const root = mount(store, <><Sidebar searchOpen={false} onSearchOpenChange={() => {}} onSettings={() => {}} /><ConfirmDialog /></>);
  const posted = () => requests.map(url => Object.fromEntries(url.searchParams));
  return {store, root, posted};
}

const click = (element: HTMLElement) => act(() => clickControl(element));
const sessionRows = (root: HTMLElement) => findAll(root, '[data-session]').map(row => row.dataset.session);
const row = (root: HTMLElement, id: string) => find(root, `[data-session="${id}"]`);
const pick = (label: string) => {
  const item = findAll(document, '[role=menuitem]').find(item => item.textContent === label);
  if (!item) throw new Error('No menu item ' + label);
  clickControl(item);
};
const showStopped = (root: HTMLElement, selector = '[data-workspace="w"] .workspace-section-header') =>
  choose(root, selector, 'Show Stopped Sessions');
const key = (element: HTMLElement, key: string) => act(() => element.dispatchEvent(new KeyboardEvent('keydown', {key, bubbles: true})));
function rename(root: HTMLElement, id: string) {
  key(row(root, id), 'F2');
  return element(root, 'input', HTMLInputElement);
}

const contextMenu = (element: HTMLElement) => act(() => element.dispatchEvent(new MouseEvent('contextmenu', {bubbles: true, cancelable: true})));
const choose = (root: HTMLElement, selector: string, label: string) => {
  contextMenu(find(root, selector));
  pick(label);
};

const dialog = () => document.querySelector('[role=alertdialog]');
const openDialog = () => find(document, '[role=alertdialog]');
const dialogReference = (attribute: string) => document.getElementById(openDialog().getAttribute(attribute) ?? '')?.textContent;
const dialogButton = (label: string) => {
  const button = findAll(openDialog(), 'button').find(b => b.textContent === label);
  if (!button) throw new Error('No dialog button ' + label);
  return button;
};

test('Enter saves a changed title, Escape cancels, and leaving the field saves', () => {
  const {root, posted} = setup([session('a')]);
  let field = rename(root, 'a');
  expect(document.activeElement).toBe(field);
  setValue(field, '  Renamed  ');
  key(field, 'Enter');
  expect(posted()).toEqual([{action: 'rename', id: 'a', title: 'Renamed'}]);
  expect(root.querySelector('input')).toBeNull();

  field = rename(root, 'a');
  setValue(field, 'Ignored');
  key(field, 'Escape');
  expect(posted().length).toBe(1);

  field = rename(root, 'a');
  setValue(field, 'Blurred');
  act(() => field.dispatchEvent(new FocusEvent('focusout', {bubbles: true})));
  expect(posted()[1]).toEqual({action: 'rename', id: 'a', title: 'Blurred'});

  field = rename(root, 'a');
  setValue(field, 'Session a');
  key(field, 'Enter');
  expect(posted().length).toBe(2);
});

test('a session\'s menu stops a running session and starts a stopped or failed one', () => {
  const {root, posted} = setup([
    session('running'),
    session('stopped', {running: false}),
    session('failed', {activity: 'exited'}),
  ]);
  showStopped(root);
  choose(root, '[data-session="running"]', 'Stop Session');
  choose(root, '[data-session="stopped"]', 'Start Session');
  choose(root, '[data-session="failed"]', 'Start Session');
  expect(posted().slice(-3)).toEqual([
    {action: 'stop', id: 'running'},
    {action: 'start', id: 'stopped'},
    {action: 'start', id: 'failed'},
  ]);
  expect(document.querySelector('[role=menu]')).toBeNull();
});

test('deleting a session asks first and posts only on Delete', () => {
  const {root, posted} = setup([session('a')]);
  choose(root, '[data-session="a"]', 'Delete Session');
  expect(openDialog().getAttribute('data-state')).toBe('open');
  expect(document.activeElement === dialogButton('Cancel')).toBe(true);
  expect(dialogReference('aria-labelledby')).toBe("Are you sure you want to delete 'Session a'?");
  expect(dialogReference('aria-describedby')).toContain('Its agent stops');
  expect(posted()).toEqual([]);
  act(() => dialogButton('Cancel').click());
  expect(dialog()).toBeNull();
  expect(posted()).toEqual([]);

  choose(root, '[data-session="a"]', 'Delete Session');
  act(() => dialogButton('Delete').click());
  expect(dialog()).toBeNull();
  expect(posted()).toEqual([{action: 'delete', id: 'a'}]);
});

test('only one row is tabbable, and the arrow keys move and open rows', () => {
  const {root, store} = setup([session('a', {children: [session('child', {parent: 'a'})]}), session('b')]);
  const tabbable = () => findAll(root, '[role="treeitem"][tabindex="0"]').map(r => r.dataset.session);
  expect(tabbable()).toEqual(['a']);
  act(() => row(root, 'a').focus());
  key(row(root, 'a'), 'ArrowLeft');
  expect(sessionRows(root)).toEqual(['a', 'b']);
  key(row(root, 'a'), 'ArrowRight');
  expect(sessionRows(root)).toEqual(['a', 'child', 'b']);
  key(row(root, 'a'), 'ArrowRight');
  expect(document.activeElement).toBe(row(root, 'child'));
  expect(tabbable()).toEqual(['child']);
  key(row(root, 'child'), 'ArrowLeft');
  expect(document.activeElement).toBe(row(root, 'a'));
  key(row(root, 'a'), 'End');
  expect(document.activeElement).toBe(row(root, 'b'));
  key(row(root, 'b'), 'Enter');
  expect(store.get().selected).toBe('b');
});
