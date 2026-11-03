import {afterEach, expect, spyOn, test} from 'bun:test';
import {act} from 'preact/test-utils';
import {mount, unmountAll} from '../../test-render.jsx';
import {openUnsent} from '../sessions/selection.js';
import {createStore} from '../store.js';
import {Composer} from './composer.jsx';
import {unsentKey} from './drafts.js';

let fetches;
afterEach(() => {
  fetches?.mockRestore();
  fetches = null;
  unmountAll();
});

const session = {id: 'a', activity: 'idle', harness: 'claude', model: 'opus', effort: 'high'};
const store = () => createStore({workspaces: [], selected: null, pending: null, drafts: {}, unsent: null, session: null});

test('a composer focuses its text box when it mounts', () => {
  const root = mount(store(), <Composer session={session} />);
  expect(document.activeElement).toBe(root.querySelector('textarea'));
  const select = root.querySelector('vscode-single-select');
  expect(select.hasAttribute('disabled')).toBe(true);
  expect(select.querySelector('vscode-option[selected]').textContent).toBe('Claude · Opus · High');
});

test('a composer leaves the focus in a tab strip', () => {
  const strip = document.createElement('div');
  strip.className = 'editor-tabs';
  const tab = document.createElement('button');
  strip.append(tab);
  document.body.append(strip);
  tab.focus();
  mount(store(), <Composer session={session} />);
  expect(document.activeElement).toBe(tab);
});

test('send is disabled without content and no stop button shows while the agent works', () => {
  const s = store();
  let root = mount(s, <Composer session={session} />);
  const send = () => root.querySelector('[title="Send"]');
  expect(send().hasAttribute('disabled')).toBe(true);
  expect(root.querySelector('[title="Stop"]')).toBeNull();
  const field = root.querySelector('textarea');
  field.value = 'hello';
  field.dispatchEvent(new Event('input'));
  expect(send().hasAttribute('disabled')).toBe(false);
  unmountAll();
  root = mount(s, <Composer session={{...session, activity: 'busy'}} />);
  expect(root.querySelector('[title="Stop"]')).toBeNull();
});

test('an ended session cannot send and shows no exited dot', () => {
  const s = store();
  s.update(state => ({...state, drafts: {[session.id]: {text: 'hello', images: [], reading: 0}}}));
  const root = mount(s, <Composer session={{...session, ended: true, activity: 'exited'}} />);
  expect(root.querySelector('[title="Send"]').hasAttribute('disabled')).toBe(true);
  expect(root.querySelector('[title="Exited"]')).toBeNull();
});

const profiles = [
  {name: 'claude', description: 'Claude for hard tasks', harness: 'claude', model: 'opus', effort: 'high'},
  {name: 'codex', description: 'Codex for ordinary tasks', harness: 'codex', model: 'sol', effort: 'medium'},
];
const unsentSession = {id: unsentKey, activity: 'idle'};
const settle = () => act(() => new Promise(resolve => setTimeout(resolve)));
const unsentStore = () => {
  const s = store();
  s.update(state => openUnsent(state, 'w'));
  return s;
};

test('the unsent composer lists profiles, defaults to the first, and keeps the draft when another is chosen', async () => {
  fetches = spyOn(globalThis, 'fetch').mockImplementation(async () => Response.json(profiles));
  const s = unsentStore();
  const root = mount(s, <Composer session={unsentSession} />);
  const field = root.querySelector('textarea');
  field.value = 'hello';
  field.dispatchEvent(new Event('input'));
  const send = () => root.querySelector('[title="Send"]');
  expect(send().hasAttribute('disabled')).toBe(true);
  await settle();

  expect(s.get().unsent.profile).toBe('claude');
  expect(send().hasAttribute('disabled')).toBe(false);
  const options = [...root.querySelectorAll('vscode-option')];
  expect(options.map(o => o.textContent)).toEqual(['Claude · Opus · High', 'Codex · Sol · Medium']);
  expect(options.map(o => o.getAttribute('description'))).toEqual(profiles.map(p => p.description));
  expect(root.querySelector('vscode-single-select').hasAttribute('disabled')).toBe(false);

  const select = root.querySelector('vscode-single-select');
  select.value = 'codex';
  act(() => select.dispatchEvent(new Event('change')));
  expect(s.get().unsent.profile).toBe('codex');
  expect(s.get().drafts[unsentKey].text).toBe('hello');
});

test('profiles that arrive for a replaced unsent session are ignored', async () => {
  const answers = [];
  fetches = spyOn(globalThis, 'fetch').mockImplementation(() => new Promise(resolve => answers.push(resolve)));
  const s = unsentStore();
  mount(s, <Composer session={unsentSession} />);
  s.update(state => openUnsent(state, 'other'));
  expect(answers.length).toBe(2);
  answers[0](Response.json([profiles[1]]));
  await settle();
  expect(s.get().unsent).toMatchObject({workspace: 'other', profile: null});
  answers[1](Response.json(profiles));
  await settle();
  expect(s.get().unsent.profile).toBe('claude');
});

test('the profile selector stays in place and disables while the first message sends', async () => {
  fetches = spyOn(globalThis, 'fetch').mockImplementation(async () => Response.json(profiles));
  const s = unsentStore();
  const root = mount(s, <Composer session={unsentSession} />);
  await settle();
  const select = root.querySelector('vscode-single-select');
  s.update(state => ({...state, unsent: {...state.unsent, sending: true}}));
  expect(root.querySelector('vscode-single-select')).toBe(select);
  expect(select.hasAttribute('disabled')).toBe(true);
  select.value = 'codex';
  act(() => select.dispatchEvent(new Event('change')));
  expect(s.get().unsent.profile).toBe('claude');
});
