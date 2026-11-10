import {afterEach, expect, mock, test} from 'bun:test';
import {act} from 'react';
import {clickControl, element, find, mount, setValue, stubFetch, unmountAll} from '../../test-render.tsx';
import {testConfig, testSession, testState, testWorkspace} from '../../test-state.ts';
import {SessionSelection} from '../agent/session-selection.tsx';
import {WorktreeSelect} from '../agent/worktree-select.tsx';
import {unsentKey as workspaceDraftKey} from '../agent/drafts.ts';
import type {ModelEntry} from '../agent/types.ts';
import {openUnsent} from '../sessions/selection.ts';
import {createSession} from '../sessions/session-actions.ts';
import {createStore, useStore} from '../store.ts';
import {SettingsDialog} from './settings-dialog.tsx';
import {openSettings} from './workbench.ts';
import type {Config} from './types.ts';

const unsentKey = workspaceDraftKey('w');

const config = testConfig({child_model_guidance: 'Use sol for reviews.'});
const sol: ModelEntry = {harness: 'opencode', model: 'sol', title: 'Sol', efforts: ['low'], default_effort: 'low'};
afterEach(() => { mock.restore(); unmountAll(); });
const settle = () => act(() => new Promise<void>(resolve => setImmediate(resolve)));
const button = (name: string) => {
  const found = [...document.querySelectorAll('button')].find(b => b.textContent === name);
  if (!found) throw new Error('Missing button: ' + name);
  return found;
};
const click = async (name: string) => { await act(async () => button(name).click()); };
const guidance = () => element(document, '#child-model-guidance', HTMLTextAreaElement);
const write = (value: string) => {
  const field = guidance();
  act(() => { setValue(field, value); field.dispatchEvent(new Event('input', {bubbles: true})); });
};
const select = (id: string, option: string) => {
  const key = (target: Element) => act(() => { target.dispatchEvent(new KeyboardEvent('keydown', {key: 'Enter', bubbles: true, cancelable: true})); });
  key(element(document, '#' + id, HTMLButtonElement));
  const found = [...document.querySelectorAll('[role=option]')].find(o => o.textContent === option);
  if (!found) throw new Error('Missing option: ' + option);
  key(found);
};
const shown = (id: string) => element(document, '#' + id, HTMLButtonElement).textContent;
function View() {
  const [, store] = useStore();
  return <><button onClick={() => store.update(s => openSettings(s))}>Open settings</button><SettingsDialog /></>;
}
const open = async () => { mount(createStore(testState({dialog: 'settings'})), <View />); await settle(); };

test('preferences save with the config and apply to the page', async () => {
  const posts: Config[] = [];
  stubFetch((url, init) => {
    if (init?.method === 'POST') { posts.push(JSON.parse(String(init.body))); return new Response(null, {status: 204}); }
    return url === 'version' ? new Response('build') : Response.json(config);
  });
  const store = createStore(openUnsent(testState({dialog: 'settings', workspaces: [testWorkspace({id: 'w'})]}), 'w'));
  const root = mount(store, <><WorktreeSelect /><View /></>);
  await settle();
  select('theme', 'Dark');
  select('terminal-font-size', '16 px');
  select('default-location', 'New worktree');
  await click('Notifications');
  await act(async () => element(document, '#notify-desktop', HTMLButtonElement).click());
  expect(element(document, '#notify-sound', HTMLButtonElement).disabled).toBe(true);
  await act(async () => element(document, '#notify-replies', HTMLButtonElement).click());
  await click('Save changes');
  const saved: Config = {...config, theme: 'dark', terminal_font_size: 16, default_worktree: true,
    notifications: {requests: true, replies: false, desktop: false, sound: true}};
  expect(posts).toEqual([saved]);
  expect(store.get().config).toEqual(saved);
  // An untouched new session follows the saved default location.
  expect(element(root, '[aria-label=Location]', HTMLButtonElement).textContent).toBe('New worktree');
});

test('guidance saves with the config', async () => {
  const posts: Config[] = [];
  stubFetch((url, init) => {
    if (init?.method === 'POST') { posts.push(JSON.parse(String(init.body))); return new Response(null, {status: 204}); }
    return url === 'version' ? new Response('build') : Response.json(config);
  });
  await open();
  await click('Agents');
  expect(guidance().value).toBe('Use sol for reviews.');
  expect(document.body.textContent).toContain('does not limit which models they can choose');
  write('Use opus for design.');
  await click('Save changes');
  expect(posts).toEqual([{...config, child_model_guidance: 'Use opus for design.'}]);
});

test('the review model and prompt save with the config', async () => {
  const posts: Config[] = [];
  stubFetch((url, init) => {
    if (init?.method === 'POST') { posts.push(JSON.parse(String(init.body))); return new Response(null, {status: 204}); }
    return url === 'version' ? new Response('build') : Response.json({...config, models: [sol]});
  });
  await open();
  await click('Review');
  const prompt = element(document, '#review-prompt', HTMLTextAreaElement);
  expect(prompt.value).toBe(config.review_prompt);
  act(() => { setValue(prompt, 'Review {url} carefully.'); prompt.dispatchEvent(new Event('input', {bubbles: true})); });
  clickControl(element(document, 'button[aria-label="Review model"]', HTMLButtonElement));
  await settle(); await settle();
  clickControl([...document.querySelectorAll<HTMLElement>('[cmdk-item]')].find(i => i.textContent === 'Sol')!);
  await settle(); await settle();
  await click('Save changes');
  expect(posts).toEqual([{...config, models: [sol], review_prompt: 'Review {url} carefully.', review_selection: {harness: 'opencode', model: 'sol', effort: 'low'}}]);
});

test('the utility model saves with the config and clears', async () => {
  const posts: Config[] = [];
  stubFetch((url, init) => {
    if (init?.method === 'POST') { posts.push(JSON.parse(String(init.body))); return new Response(null, {status: 204}); }
    return url === 'version' ? new Response('build') : Response.json({...config, models: [sol]});
  });
  await open();
  clickControl(element(document, 'button[aria-label="Utility model"]', HTMLButtonElement));
  await settle(); await settle();
  clickControl([...document.querySelectorAll<HTMLElement>('[cmdk-item]')].find(i => i.textContent === 'Sol')!);
  await settle(); await settle();
  await click('Save changes');
  expect(posts).toEqual([{...config, models: [sol], utility_selection: {harness: 'opencode', model: 'sol', effort: 'low'}}]);
});

test('Clear removes the utility model', async () => {
  const posts: Config[] = [];
  const set = {...config, models: [sol], utility_selection: {harness: 'opencode', model: 'sol', effort: 'low'}};
  stubFetch((url, init) => {
    if (init?.method === 'POST') { posts.push(JSON.parse(String(init.body))); return new Response(null, {status: 204}); }
    return url === 'version' ? new Response('build') : Response.json(set);
  });
  await open();
  await click('Clear');
  await click('Save changes');
  expect(posts).toEqual([{...config, models: [sol]}]);
});

test('cancel discards edits and reopening reloads the saved config', async () => {
  const fetches = stubFetch(url => url === 'version' ? new Response('build') : Response.json(config));
  await open();
  await click('Agents');
  write('discard me');
  await click('Cancel');
  await click('Open settings');
  await click('Agents');
  expect(guidance().value).toBe('Use sol for reviews.');
  expect(fetches.mock.calls.filter(([, init]) => init?.method === 'POST')).toHaveLength(0);
});

test('a refused save keeps the edits and shows the server reason', async () => {
  stubFetch((url, init) => init?.method === 'POST' ? new Response('default selection: missing model', {status: 400}) : url === 'version' ? new Response('build') : Response.json(config));
  await open();
  await click('Agents');
  write('kept');
  await click('Save changes');
  expect(find(document, '[role=alert]').textContent).toContain('default selection: missing model');
  expect(guidance().value).toBe('kept');
  expect(button('Save changes').disabled).toBe(false);
});

test('saving holds the modal open until the write completes', async () => {
  let complete!: (response: Response) => void;
  stubFetch((url, init) => init?.method === 'POST' ? new Promise<Response>(resolve => { complete = resolve; }) : url === 'version' ? new Response('build') : Response.json(config));
  await open();
  await click('Save changes');
  expect(button('Cancel').disabled).toBe(true);
  expect(button('Saving…').disabled).toBe(true);
  expect(document.querySelector('[data-slot=dialog-close]')).toBeNull();
  await act(async () => complete(new Response(null, {status: 204})));
  await settle();
  expect(Boolean(document.querySelector('[role=dialog][data-state=open]'))).toBe(false);
});

test('the default model is chosen without a workspace and initializes a draft that has no settings yet', async () => {
  const posts: Config[] = [];
  const created: Record<string, string>[] = [];
  stubFetch((url, init) => {
    if (url === 'config' && init?.method === 'POST') { posts.push(JSON.parse(String(init.body))); return new Response(null, {status: 204}); }
    if (url.startsWith('session?')) {
      created.push(Object.fromEntries(new URL(url, 'http://x/').searchParams));
      return Response.json({session: 'created'});
    }
    return url === 'version' ? new Response('build') : Response.json({...config, models: [sol]});
  });
  const store = createStore(testState({dialog: 'settings', config: testConfig({default_selection: undefined})}));
  const root = mount(store, <><SessionSelection session={testSession({id: unsentKey})} /><View /></>);
  await settle();
  const picker = element(document, 'button[aria-label^="Default model"]', HTMLButtonElement);
  expect(picker.title).toBe('Claude opus, effort high');
  clickControl(picker);
  await settle(); await settle();
  clickControl([...document.querySelectorAll<HTMLElement>('[cmdk-item]')].find(i => i.textContent === 'Sol')!);
  await settle(); await settle();
  await click('Save changes');
  const chosen = {harness: 'opencode', model: 'sol', effort: 'low'};
  expect(posts[0].default_selection).toEqual(chosen);
  act(() => store.update(s => openUnsent({...s, workspaces: [testWorkspace({id: 'w'})]}, 'w')));
  expect(element(root, 'button[aria-label^="Model"]', HTMLButtonElement).title).toBe('OpenCode Sol, effort low');
  await act(async () => { await createSession(store, {text: 'Start the draft', images: []}, unsentKey); });
  expect(created[0]).toMatchObject({action: 'new', ...chosen});
});

test('a model added in Agents can be the default before both are saved together', async () => {
  const posts: Config[] = [];
  stubFetch((url, init) => {
    if (url === 'config' && init?.method === 'POST') { posts.push(JSON.parse(String(init.body))); return new Response(null, {status: 204}); }
    if (url === 'harnesses?') return Response.json([{id: 'opencode'}]);
    return url === 'version' ? new Response('build') : Response.json(config);
  });
  await open();
  await click('Agents');
  await click('Add Model');
  select('model-harness', 'OpenCode');
  for (const [id, value] of [['model-id', 'sol'], ['model-title', 'Sol'], ['model-efforts', 'low']]) {
    const field = element(document, '#' + id, HTMLInputElement);
    act(() => { setValue(field, value); field.dispatchEvent(new Event('input', {bubbles: true})); });
  }
  await click('Save');
  await click('General');
  clickControl(element(document, 'button[aria-label^="Default model"]', HTMLButtonElement));
  await settle(); await settle();
  clickControl([...document.querySelectorAll<HTMLElement>('[cmdk-item]')].find(i => i.textContent === 'Sol')!);
  await settle(); await settle();
  await click('Save changes');
  expect(posts).toEqual([{...config, default_selection: {harness: 'opencode', model: 'sol', effort: 'low'}, models: [sol]}]);
});

test('the default picker opens Agents settings in the same dialog', async () => {
  stubFetch(url => url === 'version' ? new Response('build') : Response.json(config));
  await open();
  clickControl(element(document, 'button[aria-label^="Default model"]', HTMLButtonElement));
  await settle();
  await click('Open Settings');
  expect(document.querySelector('[aria-current=page]')?.textContent).toBe('Agents');
  expect(document.body.textContent).toContain('Add Model');
});
