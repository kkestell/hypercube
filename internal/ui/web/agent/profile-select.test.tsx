import {afterEach, expect, mock, test} from 'bun:test';
import {act} from 'react';
import {clickControl, element, mount, stubFetch, unmountAll} from '../../test-render.tsx';
import {testSession, testState, testWorkspace} from '../../test-state.ts';
import {NotificationStack} from '../notifications/notification-stack.tsx';
import type {Activity, Session} from '../sessions/types.ts';
import {createStore, useStore, type Store} from '../store.ts';
import {ProfileSelect} from './profile-select.tsx';
import type {Profile} from './types.ts';

afterEach(() => { mock.restore(); unmountAll(); });

const profiles: Profile[] = [
  {name: 'first', description: '', harness: 'test', model: 'old', effort: 'low'},
  {name: 'next', description: '', harness: 'test', model: 'new', effort: 'high'},
  {name: 'foreign', description: '', harness: 'other', model: 'foreign', effort: 'high'},
];
const session = testSession({id: 's', harness: 'test', model: 'saved', effort: 'medium', ready: true});
const store = (shown: Session) => createStore(testState({workspaces: [testWorkspace({id: 'w', sessions: [shown]})]}));
const changeSession = (s: Store, change: Partial<Session>) =>
  s.update(state => ({...state, workspaces: [testWorkspace({id: 'w', sessions: [{...state.workspaces[0].sessions[0], ...change}]})]}));
function View() {
  const [state] = useStore();
  return <><ProfileSelect session={state.workspaces[0].sessions[0]} /><NotificationStack /></>;
}
const settle = () => act(async () => {});
const chooseNext = async (root: HTMLElement) => {
  clickControl(element(root, '[aria-label=Profile]', HTMLButtonElement));
  await settle();
  const options = [...document.querySelectorAll('[role=menuitemradio]')];
  expect(options.map(o => o.textContent)).toEqual(['Test · Old · Low', 'Test · New · High']);
  act(() => options[1].dispatchEvent(new KeyboardEvent('keydown', {key: 'Enter', bubbles: true, cancelable: true})));
  await settle();
};

test.each<Activity>(['idle', 'busy'])('a ready %s session selects only profiles of its harness and displays saved values', async activity => {
  const fetches = stubFetch(url => url === 'profiles' ? Response.json(profiles) : new Response(''));
  const s = store({...session, activity});
  const root = mount(s, <View />);
  await settle();
  const button = element(root, '[aria-label=Profile]', HTMLButtonElement);
  expect(button.disabled).toBe(false);
  // The saved pair has no configured profile, but keeps its label.
  expect(button.textContent).toBe('Test · Saved · Medium');
  await chooseNext(root);
  const posts = fetches.mock.calls.filter(([, init]) => init?.method === 'POST');
  expect(posts).toHaveLength(1);
  expect(Object.fromEntries(new URL(posts[0][0], 'http://x/').searchParams)).toEqual({action: 'profile', id: 's', profile: 'next'});
  act(() => changeSession(s, {model: 'new', effort: 'high'}));
  expect(button.textContent).toBe('Test · New · High');
  act(() => changeSession(s, {ready: false}));
  expect(button.disabled).toBe(true);
});

test('a rejected selection stays visible through the existing error notification', async () => {
  stubFetch(url => url === 'profiles' ? Response.json(profiles) : new Response('effort refused', {status: 400}));
  const root = mount(store(session), <View />);
  await settle();
  await chooseNext(root);
  expect(element(root, '[role=alert]', HTMLElement).textContent).toContain('effort refused');
  expect(element(root, '[aria-label=Profile]', HTMLElement).textContent).toBe('Test · Saved · Medium');
});

test.each([['claude', 'Claude'], ['codex', 'Codex'], ['opencode', 'OpenCode'], ['pi', 'Pi']])('%s uses its icon in the selected profile and menu', async (harness, name) => {
  stubFetch(() => Response.json([{...profiles[0], harness, model: 'saved', effort: 'medium'}]));
  const root = mount(store({...session, harness}), <View />);
  await settle();
  const button = element(root, '[aria-label=Profile]', HTMLButtonElement);
  expect(button.textContent).toBe('Saved · Medium');
  expect(element(button, 'svg[role=img]', SVGElement).getAttribute('aria-label')).toBe(name);
  clickControl(button);
  await settle();
  const option = element(document.body, '[role=menuitemradio]', HTMLElement);
  expect(option.textContent).toBe('Saved · Medium');
  expect(element(option, 'svg[role=img]', SVGElement).getAttribute('aria-label')).toBe(name);
});
