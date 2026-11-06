import {afterEach, beforeEach, expect, mock, spyOn, test, type Mock} from 'bun:test';
import {act} from 'react';
import {clickControl, element, type FetchHandler, mount, setValue, stubFetch, unmountAll} from '../../test-render.tsx';
import {testSession, testState, testWorkspace} from '../../test-state.ts';
import {emptySession, openUnsent, unsentSending} from '../sessions/selection.ts';
import type {Merge, Session} from '../sessions/types.ts';
import type {State} from '../state.ts';
import {createStore} from '../store.ts';
import type {TranscriptItem} from '../transcript/types.ts';
import {Composer} from './composer.tsx';
import {unsentKey} from './drafts.ts';
import type {Profile} from './types.ts';

let fetches: Mock<FetchHandler>;
beforeEach(() => { fetches = stubFetch(() => Response.json([])); });
afterEach(() => {
  mock.restore();
  unmountAll();
});

const session = testSession({id: 'a', harness: 'claude', model: 'opus', effort: 'high'});
const store = () => createStore(testState());
const textarea = (root: HTMLElement) => element(root, 'textarea', HTMLTextAreaElement);
const control = (root: HTMLElement, title: string) => element(root, `[title="${title}"]`, HTMLButtonElement);
const profileButton = (root: HTMLElement) => element(root, '.composer-profile', HTMLButtonElement);
const queryOf = (url: string) => Object.fromEntries(new URL(url, 'http://x/').searchParams);
const sentBody = (init?: RequestInit): {text: string; images: unknown[]} => JSON.parse(String(init?.body));

const typeInto = (field: HTMLTextAreaElement, value: string) => {
  setValue(field, value);
  field.setSelectionRange(value.length, value.length);
  act(() => field.dispatchEvent(new Event('input', {bubbles: true})));
};
const press = (field: HTMLTextAreaElement, key: string, options: KeyboardEventInit = {}): boolean => {
  const event = new KeyboardEvent('keydown', {key, bubbles: true, cancelable: true, ...options});
  act(() => field.dispatchEvent(event));
  return event.defaultPrevented;
};
const pressAndRelease = (field: HTMLTextAreaElement, key: string) => act(() => {
  field.dispatchEvent(new KeyboardEvent('keydown', {key, bubbles: true, cancelable: true}));
  field.dispatchEvent(new KeyboardEvent('keyup', {key, bubbles: true, cancelable: true}));
});

const prompt = (id: string, text: string, from?: string): TranscriptItem => ({id, seq: Number(id), kind: 'user', text, from});
const withHistory = (state: State, ...items: TranscriptItem[]): State => ({
  ...state,
  session: {...emptySession('a'), transcript: {order: items.map(item => item.id), items: Object.fromEntries(items.map(item => [item.id, item]))}},
});

test('a composer focuses its text box when it mounts', () => {
  const root = mount(store(), <Composer session={session} />);
  expect(document.activeElement).toBe(textarea(root));
  expect(profileButton(root).disabled).toBe(true);
  expect(profileButton(root).textContent).toBe('Opus · High');
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

test('a busy agent can be stopped until text or an image is added', () => {
  const s = store();
  const root = mount(s, <Composer session={{...session, activity: 'busy'}} />);
  const field = textarea(root);
  clickControl(control(root, 'Stop'));
  expect(queryOf(fetches.mock.calls[0][0])).toEqual({action: 'interrupt', id: 'a'});
  typeInto(field, 'hello');
  expect(control(root, 'Send').disabled).toBe(false);
  typeInto(field, '  \n ');
  expect(control(root, 'Stop')).not.toBeNull();
  act(() => s.update(state => ({...state, drafts: {a: {text: '', images: [{mime_type: 'image/png', data: 'YQ=='}], reading: 0}}})));
  expect(control(root, 'Send').disabled).toBe(false);
});

test('pasting images attaches them and leaves text paste alone', async () => {
  const root = mount(store(), <Composer session={session} />);
  const field = textarea(root);
  const paste = (files: File[]) => {
    const event = new Event('paste', {bubbles: true, cancelable: true});
    Object.defineProperty(event, 'clipboardData', {value: {files}});
    act(() => field.dispatchEvent(event));
    return event;
  };
  expect(paste([]).defaultPrevented).toBe(false);
  const readers: FileReader[] = [];
  spyOn(FileReader.prototype, 'readAsDataURL').mockImplementation(function (this: FileReader) { readers.push(this); });
  expect(paste([new File(['image'], 'image.png', {type: 'image/png'})]).defaultPrevented).toBe(false);
  Object.defineProperty(readers[0], 'result', {value: 'data:image/png;base64,YQ=='});
  await act(async () => readers[0].dispatchEvent(new ProgressEvent('load')));
  expect(root.querySelector('img[alt="Attached image"]')).not.toBeNull();
});

test('a parked session can send, which starts it', () => {
  const s = store();
  s.update(state => ({...state, drafts: {[session.id]: {text: 'hello', images: [], reading: 0}}}));
  const root = mount(s, <Composer session={{...session, activity: 'exited'}} />);
  expect(control(root, 'Send').disabled).toBe(false);
});

test('an ended session cannot send and shows no exited dot', () => {
  const s = store();
  s.update(state => ({...state, drafts: {[session.id]: {text: 'hello', images: [], reading: 0}}}));
  const root = mount(s, <Composer session={{...session, ended: true, activity: 'exited'}} />);
  expect(control(root, 'Send').disabled).toBe(true);
  expect(root.querySelector('[title="Exited"]')).toBeNull();
});

const profiles: Profile[] = [
  {name: 'claude', description: 'Claude for hard tasks', harness: 'claude', model: 'opus', effort: 'high'},
  {name: 'codex', description: 'Codex for ordinary tasks', harness: 'codex', model: 'sol', effort: 'medium'},
];
const unsentSession = testSession({id: unsentKey});
const settle = () => act(() => new Promise<void>(resolve => setTimeout(resolve)));
const unsentStore = () => {
  const s = store();
  s.update(state => openUnsent(state, 'w'));
  return s;
};

test('the unsent composer uses the configured default and keeps the draft when another is chosen', async () => {
  fetches = stubFetch(() => Response.json({default_profile: 'codex', profiles}));
  const s = unsentStore();
  const root = mount(s, <Composer session={unsentSession} />);
  const field = textarea(root);
  setValue(field, 'hello');
  act(() => field.dispatchEvent(new Event('input', {bubbles: true})));
  expect(control(root, 'Send').disabled).toBe(true);
  await settle();

  expect(s.get().unsent?.profile).toBe('codex');
  expect(control(root, 'Send').disabled).toBe(false);
  const select = profileButton(root);
  expect(select.disabled).toBe(false);
  expect(select.textContent).toBe('Sol · Medium');
  clickControl(select);
  await settle();
  const options = [...document.querySelectorAll('[role=menuitemradio]')];
  expect(options.map(o => o.textContent)).toEqual(profiles.map(p => `${p.harness === 'claude' ? 'Opus · High' : 'Sol · Medium'}${p.description}`));
  act(() => options[0].dispatchEvent(new KeyboardEvent('keydown', {key: 'Enter', bubbles: true, cancelable: true})));
  expect(s.get().unsent?.profile).toBe('claude');
  expect(select.textContent).toBe('Opus · High');
  expect(s.get().drafts[unsentKey].text).toBe('hello');
});

test('profiles that arrive for a replaced unsent session are ignored', async () => {
  const answers: ((response: Response) => void)[] = [];
  fetches = stubFetch(url => url.startsWith('mention-files?') ? Response.json([]) : new Promise<Response>(resolve => { answers.push(resolve); }));
  const s = unsentStore();
  mount(s, <Composer session={unsentSession} />);
  act(() => s.update(state => openUnsent(state, 'other')));
  expect(answers.length).toBe(2);
  answers[0](Response.json({default_profile: 'codex', profiles: [profiles[1]]}));
  await settle();
  expect(s.get().unsent).toMatchObject({workspace: 'other', profile: null});
  answers[1](Response.json({default_profile: 'claude', profiles}));
  await settle();
  expect(s.get().unsent?.profile).toBe('claude');
});

test('opening the selector uses saved profiles and replaces a removed draft choice with the default', async () => {
  let config = {default_profile: 'claude', profiles};
  fetches = stubFetch(() => Response.json(config));
  const s = unsentStore();
  const root = mount(s, <Composer session={unsentSession} />);
  await settle();
  typeInto(textarea(root), 'Keep this draft');
  config = {default_profile: 'codex', profiles: [profiles[1]]};
  clickControl(profileButton(root));
  await settle();
  expect(s.get().unsent?.profile).toBe('codex');
  expect([...document.querySelectorAll('[role=menuitemradio]')].map(item => item.textContent)).toEqual(['Sol · MediumCodex for ordinary tasks']);
  expect(textarea(root).value).toBe('Keep this draft');
});

test('the profile selector stays in place and disables while the first message sends', async () => {
  fetches = stubFetch(() => Response.json({default_profile: 'claude', profiles}));
  const s = unsentStore();
  const root = mount(s, <Composer session={unsentSession} />);
  await settle();
  const select = profileButton(root);
  act(() => s.update(state => unsentSending(state, true)));
  expect(profileButton(root)).toBe(select);
  expect(select.disabled).toBe(true);
  await act(async () => { clickControl(select); });
  expect(s.get().unsent?.profile).toBe('claude');
});

const worktreeStore = (noGit: boolean, sessions: Session[] = []) => {
  const s = createStore(testState({workspaces: [testWorkspace({id: 'w', path: '/repo', noGit, sessions})]}));
  s.update(state => openUnsent(state, 'w'));
  return s;
};
const worktreeToggle = (root: HTMLElement) => root.querySelector('[aria-label=Worktree]');

test('the unsent composer offers a Worktree toggle, off, only in a Git workspace', async () => {
  fetches = stubFetch(() => Response.json({default_profile: 'claude', profiles}));
  let root = mount(worktreeStore(false), <Composer session={unsentSession} />);
  await act(async () => {});
  const toggle = element(root, '[aria-label=Worktree]', HTMLButtonElement);
  expect(toggle.getAttribute('aria-pressed')).toBe('false');
  expect(toggle.disabled).toBe(false);
  unmountAll();
  await act(async () => { root = mount(worktreeStore(true), <Composer session={unsentSession} />); });
  expect(worktreeToggle(root)).toBeNull();
});

test('turning Worktree on creates the session in a worktree and locks while sending', async () => {
  fetches = stubFetch(() => Response.json({default_profile: 'claude', profiles}));
  const s = worktreeStore(false);
  const root = mount(s, <Composer session={unsentSession} />);
  await settle();
  const toggle = element(root, '[aria-label=Worktree]', HTMLButtonElement);
  clickControl(toggle);
  expect(s.get().unsent?.worktree).toBe(true);
  const releases: (() => void)[] = [];
  fetches.mockImplementation(() => new Promise<Response>(resolve => { releases.push(() => resolve(Response.json({session: 'n'}))); }));
  const field = textarea(root);
  setValue(field, 'hello');
  act(() => field.dispatchEvent(new Event('input', {bubbles: true})));
  act(() => clickControl(control(root, 'Send')));
  expect(toggle.disabled).toBe(true);
  clickControl(toggle);
  expect(s.get().unsent?.worktree).toBe(true);
  const params = new URL(fetches.mock.calls.at(-1)?.[0] ?? '', 'http://x/').searchParams;
  expect(params.get('action')).toBe('new');
  expect(params.get('worktree')).toBe('1');
  releases[0]();
  await settle();
});

const merge = (extra: Partial<Merge> = {}): Merge => ({branch: 'hypercube/session-a', target: 'main', unmerged: 0, committed: false, uncommitted: 0, ...extra});
const existing = (extra: Partial<Session> = {}) => {
  const top = testSession({...session, worktree: true, merge: merge(), children: [testSession({id: 'c', parent: 'a'})], ...extra});
  return {top, store: worktreeStore(false, [top])};
};
const mergeButton = (root: HTMLElement) => root.querySelector('.composer-merge');

test('existing sessions have no worktree toggle', () => {
  const {top, store: s} = existing();
  let root = mount(s, <Composer session={top} />);
  expect(worktreeToggle(root)).toBeNull();
  for (const shown of [{...top, worktree: false, merge: undefined}, top.children[0]]) {
    unmountAll();
    root = mount(s, <Composer session={shown} />);
    expect(worktreeToggle(root)).toBeNull();
  }
});

test('Merge shows only with unmerged commits and no error', () => {
  for (const m of [merge(), merge({committed: true}), merge({uncommitted: 1}), merge({unmerged: 1, error: 'x'})]) {
    const {top, store: s} = existing({merge: m});
    expect(mergeButton(mount(s, <Composer session={top} />))).toBeNull();
    unmountAll();
  }
});

test('Merge asks the agent to merge into the target in the checkout', async () => {
  fetches = stubFetch(() => new Response(''));
  const {top, store: s} = existing({merge: merge({unmerged: 2, committed: true})});
  const root = mount(s, <Composer session={top} />);
  const button = element(root, '.composer-merge', HTMLButtonElement);
  expect(button.disabled).toBe(false);
  await act(async () => button.click());
  const requests = fetches.mock.calls.filter(([url]) => url.startsWith('session?'));
  expect(requests).toHaveLength(1);
  const [[url, init]] = requests;
  expect(queryOf(url)).toEqual({action: 'input', id: 'a'});
  expect(sentBody(init)).toEqual({
    text: 'Commit any remaining work, then merge your branch into main in the checkout at /repo, resolving any conflicts, and report the result.',
    images: [],
  });
});

test('prompt history returns to the unsent draft', () => {
  const s = store();
  s.update(state => withHistory(state, prompt('1', 'first'), prompt('2', 'peer', 'peer'), prompt('3', 'last')));
  const root = mount(s, <Composer session={session} />);
  const field = textarea(root);
  typeInto(field, 'draft');
  press(field, 'ArrowUp'); expect(field.value).toBe('last');
  press(field, 'ArrowUp'); expect(field.value).toBe('first');
  press(field, 'ArrowDown'); expect(field.value).toBe('last');
  press(field, 'ArrowDown'); expect(field.value).toBe('draft');
});

test('file mentions filter paths and insert the selected file', async () => {
  fetches = stubFetch(() => Response.json(['internal/ui/files.go', 'README.md']));
  const root = mount(store(), <Composer session={session} />);
  const field = textarea(root);
  setValue(field, 'look @iuf'); field.setSelectionRange(field.value.length, field.value.length);
  act(() => field.dispatchEvent(new Event('input', {bubbles: true})));
  await settle();
  expect([...root.querySelectorAll('[role=option]')].map(e => e.textContent)).toEqual(['internal/ui/files.go']);
  expect(element(root, '[role=option]', HTMLElement).id).toBe(field.getAttribute('aria-activedescendant') ?? '');
  expect(element(root, '[role=listbox]', HTMLElement).id).toBe(field.getAttribute('aria-controls') ?? '');
  press(field, 'Tab');
  expect(field.value).toBe('look @internal/ui/files.go');
  expect(root.querySelector('[role=listbox]')).toBeNull();
});

test('mention follows the caret, replaces the whole token, and Escape closes it', async () => {
  fetches.mockImplementation(() => Response.json(['src/file.go']));
  const root = mount(store(), <Composer session={session} />);
  await settle();
  const field = textarea(root);
  typeInto(field, 'hello @src');
  await settle();
  act(() => { field.setSelectionRange(0, 0); field.dispatchEvent(new KeyboardEvent('keyup', {key: 'Home', bubbles: true})); });
  expect([field.selectionStart, root.querySelector('[role=listbox]')?.textContent]).toEqual([0, undefined]);
  typeInto(field, '@sr');
  act(() => { field.setSelectionRange(2, 2); field.dispatchEvent(new KeyboardEvent('keyup', {key: 'ArrowLeft', bubbles: true})); });
  press(field, 'Enter');
  expect(field.value).toBe('@src/file.go');
  expect(field.selectionStart).toBe(field.value.length);
  typeInto(field, '@src'); press(field, 'Escape');
  expect(root.querySelector('[role=listbox]')).toBeNull();
});

test('mention keys respect modifiers and Enter sends only without a selection', async () => {
  const calls: (RequestInit | undefined)[] = [];
  fetches.mockImplementation((url, init) => {
    if (url.startsWith('mention-files?')) return Response.json(['src/a.go']);
    calls.push(init);
    return new Response('');
  });
  const root = mount(store(), <Composer session={session} />);
  await settle();
  const field = textarea(root);
  typeInto(field, '@src');
  await settle();
  expect(press(field, 'Enter', {shiftKey: true})).toBe(false);
  expect(press(field, 'Tab', {shiftKey: true})).toBe(false);
  expect(press(field, 'Enter')).toBe(true);
  expect(field.value).toBe('@src/a.go');
  expect(calls).toHaveLength(0);
  typeInto(field, '@missing');
  expect(root.querySelector('[role=listbox]')).toBeNull();
  expect(press(field, 'Enter')).toBe(true);
  expect(calls).toHaveLength(1);
});

test('multiline history preserves the draft and edits recalled copies', () => {
  const s = store();
  s.update(state => withHistory(state, prompt('1', 'old\nsecond')));
  const root = mount(s, <Composer session={session} />);
  const field = textarea(root);
  typeInto(field, 'draft\nline');
  expect(press(field, 'ArrowUp')).toBe(false);
  field.setSelectionRange(0, 0); expect(press(field, 'ArrowUp')).toBe(true);
  expect(field.value).toBe('old\nsecond');
  typeInto(field, 'edited');
  expect(s.get().drafts.a.text).toBe('draft\nline');
  expect(press(field, 'ArrowDown')).toBe(true);
  expect(field.value).toBe('draft\nline');
  field.setSelectionRange(0, 0); press(field, 'ArrowUp');
  expect(field.value).toBe('old\nsecond');
});

test('sending an edited history copy clears the saved draft', async () => {
  const sent: string[] = [];
  fetches.mockImplementation((url, init) => { sent.push(sentBody(init).text); return new Response(''); });
  const s = store();
  s.update(state => withHistory(state, prompt('1', 'old')));
  const root = mount(s, <Composer session={session} />);
  const field = textarea(root);
  typeInto(field, 'draft'); press(field, 'ArrowUp'); typeInto(field, 'edited');
  expect(s.get().drafts.a.text).toBe('draft');
  press(field, 'Enter');
  await settle();
  expect(sent).toEqual(['edited']);
  expect(s.get().drafts.a).toBeUndefined();
  expect(field.value).toBe('');
});

test('arrow keyup keeps the selected file and scrolls it into view', async () => {
  fetches.mockImplementation(() => Response.json(['a.go', 'b.go', 'c.go']));
  const scrolled: [string | null, boolean | ScrollIntoViewOptions | undefined][] = [];
  spyOn(Element.prototype, 'scrollIntoView').mockImplementation(function (this: Element, options) { scrolled.push([this.textContent, options]); });
  const root = mount(store(), <Composer session={session} />);
  const field = textarea(root);
  setValue(field, '@'); field.setSelectionRange(1, 1);
  act(() => field.dispatchEvent(new Event('input', {bubbles: true})));
  await settle();
  pressAndRelease(field, 'ArrowDown'); pressAndRelease(field, 'ArrowDown');
  expect(element(root, '[aria-selected=true]', HTMLElement).textContent).toBe('c.go');
  expect(scrolled.some(([name, options]) => name === 'c.go' && typeof options === 'object' && options.block === 'nearest')).toBe(true);
  pressAndRelease(field, 'Enter');
  expect(field.value).toBe('@c.go');
});

test('opening mentions refreshes files, keeps old results while loading, and reports errors', async () => {
  const requests: {resolve: (response: Response) => void; reject: (error: Error) => void; signal?: AbortSignal | null}[] = [];
  fetches.mockImplementation((url, init) => new Promise<Response>((resolve, reject) => { requests.push({resolve, reject, signal: init?.signal}); }));
  const root = mount(store(), <Composer session={session} />);
  const field = textarea(root);
  typeInto(field, '@'); await settle();
  expect(requests).toHaveLength(1);
  requests[0].resolve(Response.json(['old.go'])); await settle();
  typeInto(field, '@o'); await settle();
  expect(requests).toHaveLength(1);
  pressAndRelease(field, 'Escape');
  typeInto(field, 'x @'); await settle();
  expect(requests).toHaveLength(2);
  expect(root.textContent).toContain('old.go');
  pressAndRelease(field, 'Escape');
  expect(requests[1].signal?.aborted).toBe(true);
  typeInto(field, 'y @'); await settle();
  expect(requests).toHaveLength(3);
  requests[2].reject(new Error('offline')); await settle();
  expect(root.textContent).toContain('Could not list files: offline');
});

test('clearing the new session workspace removes its file suggestions', async () => {
  fetches.mockImplementation(url => Response.json(url.startsWith('mention-files?') ? ['old.go'] : {default_profile: 'claude', profiles}));
  const s = unsentStore();
  const root = mount(s, <Composer session={unsentSession} />);
  const field = textarea(root);
  setValue(field, '@'); field.setSelectionRange(1, 1);
  act(() => field.dispatchEvent(new Event('input', {bubbles: true})));
  await settle();
  expect(root.textContent).toContain('old.go');
  act(() => s.update(state => ({...state, unsent: state.unsent && {...state.unsent, workspace: null}})));
  await settle();
  expect(root.textContent).not.toContain('old.go');
});

test('history ending in a mention keeps the picker closed for navigation and sending', async () => {
  const sent: string[] = [], listings: string[] = [];
  fetches.mockImplementation((url, init) => {
    if (url.startsWith('mention-files?')) { listings.push(url); return Response.json(['a.go', 'another.go']); }
    sent.push(sentBody(init).text);
    return new Response('');
  });
  const s = store();
  s.update(state => withHistory(state, prompt('1', 'older'), prompt('2', 'see @a.go')));
  const root = mount(s, <Composer session={session} />);
  const field = textarea(root);
  pressAndRelease(field, 'ArrowUp');
  expect(field.value).toBe('see @a.go');
  expect(field.selectionStart).toBe(field.value.length);
  await settle();
  expect(root.querySelector('[role=listbox]')).toBeNull();
  expect(listings).toHaveLength(0);
  pressAndRelease(field, 'ArrowUp');
  expect(field.value).toBe('older');
  pressAndRelease(field, 'ArrowDown');
  expect(field.value).toBe('see @a.go');
  pressAndRelease(field, 'Enter');
  await settle();
  expect(sent).toEqual(['see @a.go']);
});

test('Enter while composing text does not send a prompt', () => {
  const s = store();
  const root = mount(s, <Composer session={session} />);
  const field = textarea(root);
  setValue(field, 'こんにちは');
  act(() => field.dispatchEvent(new Event('input', {bubbles: true})));
  act(() => field.dispatchEvent(new KeyboardEvent('keydown', {key: 'Enter', isComposing: true, bubbles: true, cancelable: true})));
  expect(fetches).not.toHaveBeenCalled();
  expect(s.get().drafts.a.text).toBe('こんにちは');
});
