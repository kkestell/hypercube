import {afterEach, expect, mock, spyOn, test} from 'bun:test';
import {act} from 'react';
import {clickControl, element, find, findAll, mount, setValue, unmountAll} from '../../test-render.tsx';
import {testSession, testState} from '../../test-state.ts';
import {emptySession} from '../sessions/selection.ts';
import {createStore} from '../store.ts';
import {Transcript} from './transcript.tsx';
import {receivedTranscript} from './transcripts.ts';
import type {Item, WorkState} from './types.ts';

afterEach(() => {
  mock.restore();
  unmountAll();
});

// requestParams are the query parameters of the nth fetch.
const requestParams = (calls: Parameters<typeof fetch>[], n: number) => new URL(String(calls[n][0]), 'http://x/').searchParams;

const busy = testSession({id: 's', running: true, activity: 'busy'});

type NewItem = Item & {seq?: number};

function setup(items: NewItem[], session = busy) {
  let seq = 0;
  const store = createStore(testState({session: emptySession('s')}));
  const send = (items: NewItem[], reset = false) => act(() => store.update(s => receivedTranscript(s, {type: 'transcript', session: 's', reset, items: items.map(item => ({...item, seq: item.seq ?? ++seq}))})));
  send(items, true);
  const root = mount(store, <Transcript session={session} />);
  return {store, root, send};
}

const marker = (id: string, state: WorkState, at: number, final?: string[]): Item => ({id: 'L1:' + id, kind: 'state', state, at, done: true, final});

const choose = (root: HTMLElement, choice: string) => {
  act(() => clickControl(find(root, `button[data-choice="${choice}"]`)));
};

test('a proposed child stays visible after the final response and sends only its saved ID', async () => {
  const reply = Promise.withResolvers<Response>();
  const fetches = spyOn(globalThis, 'fetch').mockReturnValue(reply.promise);
  const {root, send} = setup([
    {id: 'L1:input', kind: 'user', text: 'Review this'},
    marker('working', 'working', 1),
    {id: 'L1:proposal', kind: 'proposal', text: 'Review separately', input: {name: 'review', title: 'Review separately', profile: {name: 'worker', harness: 'codex', model: 'm', effort: 'low'}, dir: '.', message: 'Check the parser'}},
    {id: 'L1:answer', kind: 'text', text: 'I found a separate task.', done: true},
    marker('idle', 'idle', 2, ['L1:answer']),
  ]);
  const card = find(root, '[aria-label="Proposed child session"]');
  expect(card.textContent).toContain('Check the parser');
  expect(card.textContent).toContain('Fresh context');
  act(() => find(card, 'button').click());
  expect(find(card, 'button').hasAttribute('disabled')).toBe(true);
  const params = requestParams(fetches.mock.calls, 0);
  expect(Object.fromEntries(params)).toEqual({action: 'start-proposal', id: 's', proposal: 'L1:proposal'});
  expect(fetches.mock.calls[0][1]?.body).toBeUndefined();
  await act(async () => reply.resolve(new Response('')));
  send([{id: 'L1:proposal', kind: 'proposal', text: 'Review separately', input: {name: 'review', title: 'Review separately', profile: {name: 'worker', harness: 'codex', model: 'm', effort: 'low'}}, done: true, answer: 'child-id'}]);
  expect(card.textContent).toContain('Child session: review');
  expect(card.querySelector('button')).toBeNull();
});

test('a refused proposal action can be retried or dismissed', async () => {
  const fetches = spyOn(globalThis, 'fetch').mockResolvedValue(new Response('Could not start', {status: 400}));
  const {root} = setup([{id: 'p', kind: 'proposal', text: 'Task', input: {name: 'child', title: 'Task', profile: {name: 'worker', harness: 'codex', model: 'm', effort: 'low'}}}]);
  const card = find(root, '[aria-label="Proposed child session"]');
  await act(async () => find(card, 'button').click());
  expect(find(card, 'button').hasAttribute('disabled')).toBe(false);
  fetches.mockResolvedValue(new Response(''));
  await act(async () => findAll(card, 'button')[1].click());
  expect(requestParams(fetches.mock.calls, 1).get('action')).toBe('dismiss-proposal');
});

test('items replace themselves in place, and unchanged items are left alone', () => {
  const {root, send} = setup([{id: '1', kind: 'user', text: 'hi', seq: 1}, {id: '2', kind: 'text', text: 'Hel', seq: 2}]);
  const list = find(root, '.transcript-list');
  const [user, reply] = list.children;
  expect(user.textContent).toBe('hi');
  expect(reply.textContent).toBe('Hel');
  send([{id: '2', kind: 'text', text: 'Hello **there**', seq: 2}, {id: '3', kind: 'text', text: 'next', seq: 3}]);
  expect(list.children.length).toBe(3);
  expect(list.children[0]).toBe(user);
  expect(list.children[1]).toBe(reply);
  expect(reply.textContent).toBe('Hello there');
  expect(reply.querySelector('strong')).not.toBeNull();
});

test('tool, error, Thinking, and prompt text is shown as text', () => {
  const {root} = setup([
    {id: 'c', kind: 'tool', tool: '<b>tool</b>', input: {command: '<script>x</script>'}, output: '<img src=x onerror=1>', done: true},
    {id: 'e', kind: 'error', text: '<u>bad</u>'},
    {id: 'r', kind: 'reasoning', text: '<i>thinking</i>'},
    {id: 'u', kind: 'user', text: '<a href=x>me</a>'},
  ]);
  act(() => find(root, '.tool-items-call > [data-slot=collapsible-trigger]').click());
  expect(root.querySelectorAll('b, script, img, u, i, a').length).toBe(0);
  for (const text of ['<b>tool</b>', '<img src=x onerror=1>', '<u>bad</u>', '<i>thinking</i>', '<a href=x>me</a>']) expect(root.textContent).toContain(text);
});

test('an attention request offers its choices, waits while sending, and opens again when refused', async () => {
  const refusal = Promise.withResolvers<Response>();
  const fetches = spyOn(globalThis, 'fetch').mockReturnValue(refusal.promise);
  const {root, send} = setup([{id: 'q', kind: 'attention', text: 'Proceed?', choices: ['yes', 'no']}]);
  const rows = () => findAll(root, 'button[data-choice]');
  const list = () => find(root, '[role="group"]');
  expect(list().getAttribute('aria-label')).toBe('Choices');
  expect(rows().map(r => r.textContent)).toEqual(['yes', 'no']);
  expect(root.querySelector('input')).toBeNull();
  choose(root, 'yes');
  expect(Object.fromEntries(requestParams(fetches.mock.calls, 0))).toEqual({action: 'answer', id: 's', request: 'q', choice: 'yes'});
  expect(rows().every(row => row.hasAttribute('disabled'))).toBe(true);
  choose(root, 'no');
  expect(fetches).toHaveBeenCalledTimes(1);
  await act(async () => {
    refusal.resolve(new Response('no longer open', {status: 409}));
    await new Promise(resolve => setTimeout(resolve));
  });
  expect(rows().every(row => !row.hasAttribute('disabled'))).toBe(true);

  send([{id: 'q', kind: 'attention', text: 'Proceed?', done: true, answer: 'no', seq: 1}]);
  expect(rows()).toEqual([]);
  expect(find(root, '.attention-item-answered').textContent).toBe('Proceed?');
  expect(find(root, '.attention-item-answer').textContent).toBe('no');
});

test('an approval request offers to always allow its tool in either scope', async () => {
  const fetches = spyOn(globalThis, 'fetch').mockResolvedValue(new Response(''));
  for (const [i, scope] of ['session', 'all'].entries()) {
    const {root} = setup([{id: 'q', kind: 'attention', text: 'Allow Bash?', choices: ['Allow', 'Deny'], grant: 'Bash'}]);
    const buttons = findAll(root, 'button[data-always]');
    expect(buttons.map(b => b.textContent)).toEqual(['Always allow in this session', 'Always allow in all sessions']);
    await act(async () => { clickControl(buttons[i]); });
    expect(Object.fromEntries(requestParams(fetches.mock.calls, i))).toEqual({action: 'allow', id: 's', request: 'q', scope});
  }
});

test('a typed answer is sent with Enter and not when empty', async () => {
  const fetches = spyOn(globalThis, 'fetch').mockResolvedValue(new Response(''));
  const {root} = setup([{id: 'q', kind: 'attention', text: 'Name?', free: true, choices: ['Ada']}]);
  const field = element(root, 'input', HTMLInputElement);
  const sendButton = find(root, '.attention-form-entry button');
  const label = find(root, 'label');
  expect(label.textContent).toBe('Other response');
  expect(label.getAttribute('for')).toBe(field.id);
  expect(field.querySelector('button')).toBeNull();
  expect(sendButton.textContent).toBe('Send');
  expect(sendButton.hasAttribute('disabled')).toBe(true);
  act(() => clickControl(sendButton));
  expect(fetches).not.toHaveBeenCalled();
  act(() => field.dispatchEvent(new KeyboardEvent('keydown', {key: 'Enter', bubbles: true})));
  expect(fetches).not.toHaveBeenCalled();
  setValue(field, 'Ada');
  act(() => field.dispatchEvent(new Event('input', {bubbles: true})));
  expect(sendButton.hasAttribute('disabled')).toBe(false);
  await act(async () => field.dispatchEvent(new KeyboardEvent('keydown', {key: 'Enter', bubbles: true})));
  expect(requestParams(fetches.mock.calls, 0).get('choice')).toBe('');
  expect(fetches.mock.calls[0][1]?.body).toBe('Ada');
});

test('a nested request stays pinned outside a collapsed activity and returns to history when answered', () => {
  const {root, send} = setup([
    {id: 'u', kind: 'user', text: 'Go', at: 1000},
    marker('w', 'working', 1100),
    {id: 'a', kind: 'tool', tool: 'agent', at: 1200},
    {id: 'q', parent: 'a', kind: 'attention', text: 'Allow?', choices: ['ok'], at: 1300},
    {id: 't', kind: 'text', text: 'Waiting on you', at: 1400},
    marker('i', 'idle', 2000, ['t']),
  ]);
  const group = find(root, '.activity-item');
  expect(group.getAttribute('data-state') === 'open').toBe(false);
  const pending = find(root, '[aria-label="Pending questions"]');
  expect(pending.querySelector('button[data-choice="ok"]')).not.toBeNull();
  expect(pending.closest('.transcript-list')).toBeNull();
  expect(root.querySelector('.transcript-list .attention-item')).toBeNull();
  send([{id: 'q', parent: 'a', kind: 'attention', text: 'Allow?', done: true, answer: 'ok', at: 1300}]);
  expect(root.querySelector('.transcript-pending')).toBeNull();
  act(() => find(group, '[data-slot=collapsible-trigger]').click());
  act(() => find(group, '.tool-items-call > [data-slot=collapsible-trigger]').click());
  expect(find(group, '.attention-item-answer').textContent).toBe('ok');
});

test('pending questions and their draft stay outside the history list while output arrives', () => {
  const {root, send} = setup([
    {id: 'q1', kind: 'attention', text: 'First?', free: true},
    {id: 'q2', kind: 'attention', text: 'Second?', choices: ['yes']},
  ]);
  const pending = find(root, '.transcript-pending');
  const field = element(pending, 'input', HTMLInputElement);
  setValue(field, 'My answer');
  act(() => field.dispatchEvent(new Event('input', {bubbles: true})));
  const view = find(root, '.transcript');
  view.scrollTop = 0;
  act(() => view.dispatchEvent(new Event('scroll')));
  send([{id: 'output', kind: 'text', text: 'More work'}]);
  expect(find(root, '.transcript-pending') === pending).toBe(true);
  expect(find(pending, 'input') === field).toBe(true);
  expect(field.value).toBe('My answer');
  expect(findAll(pending, '.attention-item-question').map(item => item.textContent)).toEqual(['First?', 'Second?']);
  expect(find(view, '.transcript-list').textContent).toBe('More work');
  expect(pending.parentElement === find(view, '.transcript-bottom')).toBe(true);
  send([{id: 'q1', kind: 'attention', text: 'First?', done: true, answer: 'My answer', seq: 0}]);
  expect(find(view, '.attention-item-answer').textContent).toBe('My answer');
  expect(pending.querySelectorAll('.attention-item').length).toBe(1);
});
