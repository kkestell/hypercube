import {afterEach, expect, spyOn, test} from 'bun:test';
import {act} from 'react';
import {clickControl, mount, unmountAll} from '../../test-render.jsx';
import {NotificationStack} from '../notifications/notification-stack.jsx';
import {createStore, useStore} from '../store.js';
import {ProfileSelect} from './profile-select.jsx';

let fetches;
afterEach(() => { fetches?.mockRestore(); unmountAll(); });

const profiles = [
  {name: 'first', harness: 'test', model: 'old', effort: 'low'},
  {name: 'next', harness: 'test', model: 'new', effort: 'high'},
  {name: 'foreign', harness: 'other', model: 'foreign', effort: 'high'},
];
const session = {id: 's', harness: 'test', model: 'saved', effort: 'medium', ready: true, activity: 'idle'};
const store = selection => createStore({selection, unsent: null, errors: [], requests: [], workspaces: [], dismissed: {}, overlays: {}});
function View() {
  const [state] = useStore();
  return <><ProfileSelect session={state.selection} /><NotificationStack /></>;
}
const settle = () => act(async () => {});
const chooseNext = async root => {
  clickControl(root.querySelector('[aria-label=Profile]'));
  await settle();
  const options = [...document.querySelectorAll('[role=menuitemradio]')];
  expect(options.map(o => o.textContent)).toEqual(['Test · Old · Low', 'Test · New · High']);
  act(() => options[1].dispatchEvent(new KeyboardEvent('keydown', {key: 'Enter', bubbles: true, cancelable: true})));
  await settle();
};

test.each(['idle', 'busy'])('a ready %s session selects only profiles of its harness and displays saved values', async activity => {
  fetches = spyOn(globalThis, 'fetch').mockImplementation(async (url, init) => url === 'profiles' ? Response.json(profiles) : new Response(''));
  const s = store({...session, activity});
  const root = mount(s, <View />);
  await settle();
  const button = root.querySelector('[aria-label=Profile]');
  expect(button.disabled).toBe(false);
  // The saved pair has no configured profile, but keeps its label.
  expect(button.textContent).toBe('Test · Saved · Medium');
  await chooseNext(root);
  const [url, init] = fetches.mock.calls.find(([, init]) => init?.method === 'POST');
  expect(Object.fromEntries(new URL(url, 'http://x/').searchParams)).toEqual({action: 'profile', id: 's', profile: 'next'});
  expect(init.method).toBe('POST');
  act(() => s.update(state => ({...state, selection: {...state.selection, model: 'new', effort: 'high'}})));
  expect(button.textContent).toBe('Test · New · High');
  act(() => s.update(state => ({...state, selection: {...state.selection, ready: false}})));
  expect(button.disabled).toBe(true);
});

test('a rejected selection stays visible through the existing error notification', async () => {
  fetches = spyOn(globalThis, 'fetch').mockImplementation(async url => url === 'profiles' ? Response.json(profiles) : new Response('effort refused', {status: 400}));
  const root = mount(store(session), <View />);
  await settle();
  await chooseNext(root);
  expect(root.querySelector('[role=alert]').textContent).toContain('effort refused');
  expect(root.querySelector('[aria-label=Profile]').textContent).toBe('Test · Saved · Medium');
});
