import {afterEach, beforeEach, expect, spyOn, test} from 'bun:test';
import {act} from 'preact/test-utils';
import {mount, unmountAll} from '../../test-render.jsx';
import {openUnsent} from '../sessions/selection.js';
import {createStore} from '../store.js';
import {Composer} from './composer.jsx';
import {unsentKey} from './drafts.js';

let fetches;
beforeEach(() => { fetches = spyOn(globalThis, 'fetch').mockImplementation(async () => Response.json([])); });
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
  expect(root.querySelector('vscode-single-select')).toBeNull();
  expect(root.querySelector('.composer-profile-label').textContent).toBe('Claude · Opus · High');
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
  fetches = spyOn(globalThis, 'fetch').mockImplementation(url => String(url).startsWith('mention-files?') ? Promise.resolve(Response.json([])) : new Promise(resolve => answers.push(resolve)));
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

const worktreeStore = (noGit, sessions = []) => {
  const s = createStore({workspaces: [{id: 'w', path: '/repo', noGit, sessions}], selected: null, pending: null, drafts: {}, unsent: null, session: null, errors: []});
  s.update(state => openUnsent(state, 'w'));
  return s;
};
const worktreeToggle = root => root.querySelector('vscode-checkbox');

test('the unsent composer offers a Worktree toggle, off, only in a Git workspace', () => {
  fetches = spyOn(globalThis, 'fetch').mockImplementation(async () => Response.json(profiles));
  let root = mount(worktreeStore(false), <Composer session={unsentSession} />);
  expect(worktreeToggle(root).hasAttribute('toggle')).toBe(true);
  expect(worktreeToggle(root).getAttribute('label')).toBe('Worktree');
  expect(worktreeToggle(root).hasAttribute('checked')).toBe(false);
  expect(worktreeToggle(root).hasAttribute('disabled')).toBe(false);
  unmountAll();
  root = mount(worktreeStore(true), <Composer session={unsentSession} />);
  expect(worktreeToggle(root)).toBeNull();
});

test('turning Worktree on creates the session in a worktree and locks while sending', async () => {
  fetches = spyOn(globalThis, 'fetch').mockImplementation(async () => Response.json(profiles));
  const s = worktreeStore(false);
  const root = mount(s, <Composer session={unsentSession} />);
  await settle();
  const toggle = worktreeToggle(root);
  toggle.checked = true;
  act(() => toggle.dispatchEvent(new Event('change')));
  expect(s.get().unsent.worktree).toBe(true);
  let release;
  fetches.mockImplementation(() => new Promise(resolve => { release = () => resolve(Response.json({session: 'n'})); }));
  const field = root.querySelector('textarea');
  field.value = 'hello';
  field.dispatchEvent(new Event('input'));
  act(() => root.querySelector('[title="Send"]').click());
  expect(worktreeToggle(root).hasAttribute('disabled')).toBe(true);
  toggle.checked = false;
  act(() => toggle.dispatchEvent(new Event('change')));
  expect(s.get().unsent.worktree).toBe(true);
  const params = new URL(fetches.mock.calls.at(-1)[0], 'http://x/').searchParams;
  expect(params.get('action')).toBe('new');
  expect(params.get('worktree')).toBe('1');
  release();
  await settle();
});

const merge = extra => ({branch: 'hypercube/session-a', target: 'main', unmerged: 0, committed: false, uncommitted: 0, ...extra});
const existing = (extra = {}) => {
  const top = {...session, running: true, worktree: true, merge: merge(), children: [{id: 'c', parent: 'a', activity: 'idle'}], ...extra};
  return {top, store: worktreeStore(false, [top])};
};
const branch = root => root.querySelector('.composer-branch');
const mergeButton = root => root.querySelector('.composer-merge');

test('an existing worktree session shows its branch and no toggle; checkout sessions and children show neither', () => {
  const {top, store: s} = existing();
  let root = mount(s, <Composer session={top} />);
  expect(worktreeToggle(root)).toBeNull();
  expect(branch(root).querySelector('vscode-icon').getAttribute('name')).toBe('git-branch');
  expect(branch(root).textContent).toBe('hypercube/session-a');
  for (const shown of [{...top, worktree: false, merge: undefined}, top.children[0]]) {
    unmountAll();
    root = mount(s, <Composer session={shown} />);
    expect(worktreeToggle(root)).toBeNull();
    expect(branch(root)).toBeNull();
  }
});

test('Merge shows only with unmerged commits and no error, and disables while stopped', () => {
  for (const m of [merge(), merge({committed: true}), merge({uncommitted: 1}), merge({unmerged: 1, error: 'x'})]) {
    const {top, store: s} = existing({merge: m});
    expect(mergeButton(mount(s, <Composer session={top} />))).toBeNull();
    unmountAll();
  }
  const {top, store: s} = existing({running: false, merge: merge({unmerged: 1, committed: true})});
  expect(mergeButton(mount(s, <Composer session={top} />)).hasAttribute('disabled')).toBe(true);
});

test('Merge asks the agent to merge into the target in the checkout', async () => {
  fetches = spyOn(globalThis, 'fetch').mockImplementation(async () => new Response(''));
  const {top, store: s} = existing({merge: merge({unmerged: 2, committed: true})});
  const root = mount(s, <Composer session={top} />);
  expect(mergeButton(root).hasAttribute('disabled')).toBe(false);
  await act(() => mergeButton(root).click());
  const [url, init] = fetches.mock.calls.find(([url]) => String(url).startsWith('session?'));
  expect(Object.fromEntries(new URL(url, 'http://x/').searchParams)).toEqual({action: 'input', id: 'a'});
  expect(JSON.parse(init.body)).toEqual({
    text: 'Commit any remaining work, then merge your branch into main in the checkout at /repo, resolving any conflicts, and report the result.',
    images: [],
  });
});

test('prompt history returns to the unsent draft', () => {
  const s = store();
  s.update(state => ({...state, session: {id: 'a', transcript: {order: ['1', '2', '3'], items: {
    1: {kind: 'user', text: 'first'}, 2: {kind: 'user', from: 'peer', text: 'peer'}, 3: {kind: 'user', text: 'last'},
  }}}}));
  const root = mount(s, <Composer session={session} />);
  const field = root.querySelector('textarea');
  field.value = 'draft'; field.dispatchEvent(new Event('input'));
  const press = key => act(() => field.dispatchEvent(new KeyboardEvent('keydown', {key, bubbles: true, cancelable: true})));
  press('ArrowUp'); expect(field.value).toBe('last');
  press('ArrowUp'); expect(field.value).toBe('first');
  press('ArrowDown'); expect(field.value).toBe('last');
  press('ArrowDown'); expect(field.value).toBe('draft');
});

test('file mentions filter paths and insert the selected file', async () => {
  fetches = spyOn(globalThis, 'fetch').mockImplementation(async () => Response.json(['internal/ui/files.go', 'README.md']));
  const root = mount(store(), <Composer session={session} />);
  const field = root.querySelector('textarea');
  field.value = 'look @iuf'; field.setSelectionRange(field.value.length, field.value.length);
  act(() => field.dispatchEvent(new Event('input')));
  await settle();
  expect([...root.querySelectorAll('[role=option]')].map(e => e.textContent)).toEqual(['internal/ui/files.go']);
  act(() => field.dispatchEvent(new KeyboardEvent('keydown', {key: 'Tab', bubbles: true, cancelable: true})));
  expect(field.value).toBe('look @internal/ui/files.go');
  expect(root.querySelector('[role=listbox]')).toBeNull();
});

test('mention follows the caret, replaces the whole token, and Escape closes it', async () => {
  fetches.mockImplementation(async () => Response.json(['src/file.go']));
  const root = mount(store(), <Composer session={session} />);
  await settle();
  const field = root.querySelector('textarea');
  const input = value => { field.value = value; field.setSelectionRange(value.length, value.length); act(() => field.dispatchEvent(new Event('input'))); };
  const press = (key, options = {}) => act(() => field.dispatchEvent(new KeyboardEvent('keydown', {key, bubbles: true, cancelable: true, ...options})));
  input('hello @src');
  await settle();
  act(() => { field.setSelectionRange(0, 0); field.dispatchEvent(new KeyboardEvent('keyup', {key: 'Home', bubbles: true})); });
  expect([field.selectionStart, root.querySelector('[role=listbox]')?.textContent]).toEqual([0, undefined]);
  input('@sr');
  act(() => { field.setSelectionRange(2, 2); field.dispatchEvent(new KeyboardEvent('keyup', {key: 'ArrowLeft', bubbles: true})); });
  press('Enter');
  expect(field.value).toBe('@src/file.go');
  expect(field.selectionStart).toBe(field.value.length);
  input('@src'); press('Escape');
  expect(root.querySelector('[role=listbox]')).toBeNull();
});

test('mention keys respect modifiers and Enter sends only without a selection', async () => {
  const calls = [];
  fetches.mockImplementation(async (url, init) => {
    if (String(url).startsWith('mention-files?')) return Response.json(['src/a.go']);
    calls.push(init); return new Response('');
  });
  const root = mount(store(), <Composer session={session} />);
  await settle();
  const field = root.querySelector('textarea');
  const input = value => { field.value = value; field.setSelectionRange(value.length, value.length); act(() => field.dispatchEvent(new Event('input'))); };
  const press = (key, options = {}) => { const e = new KeyboardEvent('keydown', {key, bubbles: true, cancelable: true, ...options}); act(() => field.dispatchEvent(e)); return e.defaultPrevented; };
  input('@src');
  await settle();
  expect(press('Enter', {shiftKey: true})).toBe(false);
  expect(press('Tab', {shiftKey: true})).toBe(false);
  expect(press('Enter')).toBe(true);
  expect(field.value).toBe('@src/a.go');
  expect(calls).toHaveLength(0);
  input('@missing');
  expect(root.querySelector('[role=listbox]')).toBeNull();
  expect(press('Enter')).toBe(true);
  expect(calls).toHaveLength(1);
});

test('multiline history preserves the draft and edits recalled copies', () => {
  const s = store();
  s.update(state => ({...state, session: {id: 'a', transcript: {order: ['1'], items: {'1': {kind: 'user', text: 'old\nsecond'}}}}}));
  const root = mount(s, <Composer session={session} />);
  const field = root.querySelector('textarea');
  const input = value => { field.value = value; field.setSelectionRange(value.length, value.length); act(() => field.dispatchEvent(new Event('input'))); };
  const press = key => { const e = new KeyboardEvent('keydown', {key, bubbles: true, cancelable: true}); act(() => field.dispatchEvent(e)); return e.defaultPrevented; };
  input('draft\nline');
  expect(press('ArrowUp')).toBe(false);
  field.setSelectionRange(0, 0); expect(press('ArrowUp')).toBe(true);
  expect(field.value).toBe('old\nsecond');
  input('edited');
  expect(s.get().drafts.a.text).toBe('draft\nline');
  expect(press('ArrowDown')).toBe(true);
  expect(field.value).toBe('draft\nline');
  field.setSelectionRange(0, 0); press('ArrowUp');
  expect(field.value).toBe('old\nsecond');
});

test('sending an edited history copy clears the saved draft', async () => {
  let sent;
  fetches.mockImplementation(async (url, init) => { sent = JSON.parse(init.body); return new Response(''); });
  const s = store();
  s.update(state => ({...state, session: {id: 'a', transcript: {order: ['1'], items: {'1': {kind: 'user', text: 'old'}}}}}));
  const root = mount(s, <Composer session={session} />);
  const field = root.querySelector('textarea');
  const input = value => { field.value = value; field.setSelectionRange(value.length, value.length); act(() => field.dispatchEvent(new Event('input'))); };
  const press = key => act(() => field.dispatchEvent(new KeyboardEvent('keydown', {key, bubbles: true, cancelable: true})));
  input('draft'); press('ArrowUp'); input('edited');
  expect(s.get().drafts.a.text).toBe('draft');
  press('Enter');
  await settle();
  expect(sent.text).toBe('edited');
  expect(s.get().drafts.a).toBeUndefined();
  expect(field.value).toBe('');
});

test('arrow keyup keeps the selected file and scrolls it into view', async () => {
  fetches.mockImplementation(async () => Response.json(['a.go', 'b.go', 'c.go']));
  const scrolled = [];
  const scroll = spyOn(Element.prototype, 'scrollIntoView').mockImplementation(function(options) { scrolled.push([this.textContent, options]); });
  try {
    const root = mount(store(), <Composer session={session} />);
    const field = root.querySelector('textarea');
    field.value = '@'; field.setSelectionRange(1, 1);
    act(() => field.dispatchEvent(new Event('input')));
    await settle();
    const key = name => act(() => {
      field.dispatchEvent(new KeyboardEvent('keydown', {key: name, bubbles: true, cancelable: true}));
      field.dispatchEvent(new KeyboardEvent('keyup', {key: name, bubbles: true, cancelable: true}));
    });
    key('ArrowDown'); key('ArrowDown');
    expect(root.querySelector('[aria-selected=true]').textContent).toBe('c.go');
    expect(scrolled.some(([name, options]) => name === 'c.go' && options?.block === 'nearest')).toBe(true);
    key('Enter');
    expect(field.value).toBe('@c.go');
  } finally { scroll.mockRestore(); }
});

test('opening mentions refreshes files, keeps old results while loading, and reports errors', async () => {
  const requests = [];
  fetches.mockImplementation((url, init) => new Promise((resolve, reject) => requests.push({resolve, reject, signal: init.signal})));
  const root = mount(store(), <Composer session={session} />);
  const field = root.querySelector('textarea');
  const input = value => { field.value = value; field.setSelectionRange(value.length, value.length); act(() => field.dispatchEvent(new Event('input'))); };
  const key = name => act(() => { field.dispatchEvent(new KeyboardEvent('keydown', {key: name, bubbles: true, cancelable: true})); field.dispatchEvent(new KeyboardEvent('keyup', {key: name, bubbles: true})); });
  input('@'); await settle();
  expect(requests).toHaveLength(1);
  requests[0].resolve(Response.json(['old.go'])); await settle();
  input('@o'); await settle();
  expect(requests).toHaveLength(1);
  key('Escape');
  input('x @'); await settle();
  expect(requests).toHaveLength(2);
  expect(root.textContent).toContain('old.go');
  key('Escape');
  expect(requests[1].signal.aborted).toBe(true);
  input('y @'); await settle();
  expect(requests).toHaveLength(3);
  requests[2].reject(new Error('offline')); await settle();
  expect(root.textContent).toContain('Could not list files: offline');
});

test('clearing the new session workspace removes its file suggestions', async () => {
  fetches.mockImplementation(async url => Response.json(String(url).startsWith('mention-files?') ? ['old.go'] : profiles));
  const s = unsentStore();
  const root = mount(s, <Composer session={unsentSession} />);
  const field = root.querySelector('textarea');
  field.value = '@'; field.setSelectionRange(1, 1);
  act(() => field.dispatchEvent(new Event('input')));
  await settle();
  expect(root.textContent).toContain('old.go');
  act(() => s.update(state => ({...state, unsent: {...state.unsent, workspace: null}})));
  await settle();
  expect(root.textContent).not.toContain('old.go');
});

test('history ending in a mention keeps the picker closed for navigation and sending', async () => {
  const sent = [], listings = [];
  fetches.mockImplementation(async (url, init) => {
    if (String(url).startsWith('mention-files?')) { listings.push(url); return Response.json(['a.go', 'another.go']); }
    sent.push(JSON.parse(init.body)); return new Response('');
  });
  const s = store();
  s.update(state => ({...state, session: {id: 'a', transcript: {order: ['1', '2'], items: {
    1: {kind: 'user', text: 'older'}, 2: {kind: 'user', text: 'see @a.go'},
  }}}}));
  const root = mount(s, <Composer session={session} />);
  const field = root.querySelector('textarea');
  const key = name => act(() => {
    field.dispatchEvent(new KeyboardEvent('keydown', {key: name, bubbles: true, cancelable: true}));
    field.dispatchEvent(new KeyboardEvent('keyup', {key: name, bubbles: true, cancelable: true}));
  });
  key('ArrowUp');
  expect(field.value).toBe('see @a.go');
  expect(field.selectionStart).toBe(field.value.length);
  await settle();
  expect(root.querySelector('[role=listbox]')).toBeNull();
  expect(listings).toHaveLength(0);
  key('ArrowUp');
  expect(field.value).toBe('older');
  key('ArrowDown');
  expect(field.value).toBe('see @a.go');
  key('Enter');
  await settle();
  expect(sent.map(prompt => prompt.text)).toEqual(['see @a.go']);
});
