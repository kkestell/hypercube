import {afterEach, expect, mock, test} from 'bun:test';
import {act, useState} from 'react';
import {clickControl, element, find, findAll, mount, setValue, stubFetch, unmountAll} from '../../test-render.tsx';
import {testSession, testState, testWorkspace} from '../../test-state.ts';
import {chooseSession} from '../sessions/session-actions.ts';
import type {Session} from '../sessions/types.ts';
import {ConfirmDialog} from '../shared/confirm-dialog.tsx';
import {createStore} from '../store.ts';
import {Sidebar} from './sidebar.tsx';

afterEach(() => {
  mock.restore();
  unmountAll();
});

const session = (id: string, extra: Partial<Session> = {}) => testSession({id, title: 'Session ' + id, ...extra});
function setup(sessions: Session[], selected: string | null = null) {
  let respond = () => new Response('');
  const requests: URL[] = [];
  stubFetch(url => {
    requests.push(new URL(url, 'http://x/'));
    return respond();
  });
  const store = createStore(testState({selected, workspaces: [testWorkspace({id: 'w', name: 'repo', path: '/repo', sessions})]}));
  function View() {
    const [searchOpen, setSearchOpen] = useState(false);
    return <><Sidebar searchOpen={searchOpen} onSearchOpenChange={setSearchOpen} onSettings={() => {}} /><ConfirmDialog /></>;
  }
  const root = mount(store, <View />);
  const posted = () => requests.map(url => Object.fromEntries(url.searchParams));
  return {store, root, posted, answer: (next: () => Response) => { respond = next; }};
}

const click = (element: HTMLElement) => act(() => clickControl(element));
const sessionRows = (root: HTMLElement) => findAll(root, '[data-session]').map(row => row.dataset.session);
const row = (root: HTMLElement, id: string) => find(root, `[data-session="${id}"]`);
const pick = (label: string) => {
  const item = findAll(document, '[role=menuitem]').find(item => item.textContent === label);
  if (!item) throw new Error('No menu item ' + label);
  clickControl(item);
};
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

test('every active family is listed with all its children, and menus offer only Archive and Unarchive', () => {
  const live = session('live', {parent: 'a'});
  const ended = session('ended', {parent: 'a', ended: true});
  const {root, posted} = setup([session('a', {activity: 'exited', children: [live, ended]}), session('b'), session('old', {ended: true})], 'live');
  expect(sessionRows(root)).toEqual(['a', 'live', 'ended', 'b']);
  expect(row(root, 'ended').className).toContain('opacity-60');
  expect(row(root, 'a').className).not.toContain('opacity-60');
  contextMenu(row(root, 'a'));
  expect(findAll(document, '[role=menuitem]').map(item => item.textContent)).toEqual(
    ['Rename', 'Open in VS Code', 'Reveal in Finder', 'Open in Terminal', 'Archive Session', 'Delete Session']);
  pick('Archive Session');
  click(find(root, '[role=tab][data-state=inactive]'));
  choose(root, '[data-session="old"]', 'Unarchive');
  expect(posted()).toEqual([{action: 'archive', id: 'a'}, {action: 'unarchive', id: 'old'}]);
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

test.each([
  {id: 'archived', view: 'Archived', other: 'Sessions'},
  {id: 'active', view: 'Sessions', other: 'Archived'},
])('search reveals an already selected $id session after switching tabs', async ({id, view, other}) => {
  const {root, store, answer} = setup([session('active'), session('archived', {ended: true})], id);
  const tabs = find(root, '[role=tablist][aria-label="Sidebar"]');
  expect(find(tabs, '[data-state=active]').getAttribute('aria-label')).toBe(view);
  click(find(tabs, '[data-state=inactive]'));
  expect(find(tabs, '[data-state=active]').getAttribute('aria-label')).toBe(other);
  expect(root.querySelector(`[data-session="${id}"]`)).toBeNull();
  answer(() => Response.json([{id, title: 'Session ' + id, workspace: 'repo', text: 'matching words'}]));
  await act(async () => clickControl(find(root, '[aria-label="Search sessions"]')));
  const input = element(document, '[cmdk-input]', HTMLInputElement);
  await act(async () => { setValue(input, 'matching '); input.dispatchEvent(new Event('input', {bubbles: true})); });
  await act(async () => clickControl(find(document, '[cmdk-item]')));
  expect(store.get().selected).toBe(id);
  expect(find(tabs, '[data-state=active]').getAttribute('aria-label')).toBe(view);
  expect(root.querySelector(`[data-session="${id}"]`)).not.toBeNull();
});

test('archive menu hides the family while search can select its chat', () => {
  const child = session('child', {parent: 'archived', ended: true});
  const {root, store, posted} = setup([session('active'), session('archived', {ended: true, children: [child]})]);
  expect(sessionRows(root)).toEqual(['active']);
  const tablists = findAll(root, '[role=tablist]');
  expect(tablists).toHaveLength(1);
  expect(findAll(tablists[0], '[role=tab]').map(tab => tab.getAttribute('aria-label'))).toEqual(['Sessions', 'Archived']);
  choose(root, '[data-session="active"]', 'Archive Session');
  expect(posted()).toEqual([{action: 'archive', id: 'active'}]);
  act(() => chooseSession(store, 'child'));
  expect(store.get().selected).toBe('child');
  expect(sessionRows(root)).toEqual(['archived', 'child']);
  click(find(root, '[role=tab][data-state=inactive]'));
  expect(sessionRows(root)).toEqual(['active']);
  click(find(root, '[role=tab][data-state=inactive]'));
  expect(sessionRows(root)).toEqual(['archived', 'child']);
  expect(posted()).toHaveLength(1);
});

test('an initially selected archived child opens its family in Archived', () => {
  const child = session('child', {parent: 'archived', ended: true});
  const {root} = setup([session('active'), session('archived', {ended: true, children: [child]})], 'child');
  expect(sessionRows(root)).toEqual(['archived', 'child']);
  expect(find(root, '[role=tablist][aria-label="Sidebar"] [data-state=active]').getAttribute('aria-label')).toBe('Archived');
});
