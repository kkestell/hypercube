import {afterEach, expect, jest, mock, spyOn, test} from 'bun:test';
import {act} from 'react';
import {clickControl, element, find, findAll, mount, setValue, stubFetch, unmountAll} from '../../test-render.tsx';
import {testSession, testState, testWorkspace} from '../../test-state.ts';
import {emptySession} from '../sessions/selection.ts';
import {createStore} from '../store.ts';
import {Transcript} from './transcript.tsx';
import {quietDelay} from './quiet-turn.tsx';
import {lineLength} from './reasoning-item.tsx';
import {receivedTranscript} from './transcripts.ts';
import type {Item, WorkState} from './types.ts';

afterEach(() => {
  mock.restore();
  unmountAll();
  jest.useRealTimers();
});

// requestParams are the query parameters of the nth fetch.
const requestParams = (calls: Parameters<typeof fetch>[], n: number) => new URL(String(calls[n][0]), 'http://x/').searchParams;

const busy = testSession({id: 's', activity: 'busy'});

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

// button is the button named name by its label or its text.
const button = (root: ParentNode, name: string) => {
  const found = findAll(root, 'button').find(b => (b.getAttribute('aria-label') ?? b.textContent) === name);
  if (!found) throw new Error(`No button named ${name}`);
  return found;
};
// labelled is the control that the label reading text names.
const labelled = (root: ParentNode, text: string) => {
  const label = findAll(root, 'label').find(l => l.textContent === text);
  const control = label && document.getElementById(label.getAttribute('for') ?? '');
  if (!control) throw new Error(`No control labelled ${text}`);
  return control;
};
// toggle is the expand and collapse button whose text includes text.
const toggle = (root: ParentNode, text: string) => {
  const found = findAll(root, 'button[aria-expanded]').find(b => b.textContent?.includes(text));
  if (!found) throw new Error(`No toggle reading ${text}`);
  return found;
};
const pendingArea = (root: ParentNode) => root.querySelector<HTMLElement>('[aria-label="Waiting on you"]');
const proposals = (root: ParentNode) => findAll(root, '[aria-label="Proposed child session"]');
const choose = (root: HTMLElement, choice: string) => {
  act(() => clickControl(labelled(root, choice)));
};
const submitButton = (root: HTMLElement) => button(root, 'Submit');
const submit = (root: HTMLElement) => act(() => clickControl(submitButton(root)));

test('an open proposal is pinned below the history, sends only its saved ID, and returns to history when started', async () => {
  const reply = Promise.withResolvers<Response>();
  const fetches = spyOn(globalThis, 'fetch').mockReturnValue(reply.promise);
  const {root, send} = setup([
    {id: 'L1:input', kind: 'user', text: 'Review this'},
    marker('working', 'working', 1),
    {id: 'L1:proposal', kind: 'proposal', text: 'Review separately', input: {name: 'review', title: 'Review separately', settings: {harness: 'codex', model: 'm', effort: 'low'}, dir: '.', message: 'Check the parser'}},
    {id: 'L1:answer', kind: 'text', text: 'I found a separate task.', done: true},
    marker('idle', 'idle', 2, ['L1:answer']),
  ]);
  const card = find(pendingArea(root)!, '[aria-label="Proposed child session"]');
  expect(proposals(root)).toEqual([card]);
  expect(card.textContent).toContain('Check the parser');
  expect(card.textContent).toContain('Fresh context');
  act(() => find(card, 'button').click());
  expect(find(card, 'button').hasAttribute('disabled')).toBe(true);
  const params = requestParams(fetches.mock.calls, 0);
  expect(Object.fromEntries(params)).toEqual({action: 'start-proposal', id: 's', proposal: 'L1:proposal'});
  expect(fetches.mock.calls[0][1]?.body).toBeUndefined();
  await act(async () => reply.resolve(new Response('')));
  send([{id: 'L1:proposal', kind: 'proposal', text: 'Review separately', input: {name: 'review', title: 'Review separately', settings: {harness: 'codex', model: 'm', effort: 'low'}}, done: true, answer: 'child-id'}]);
  expect(pendingArea(root)).toBeNull();
  const [done] = proposals(root);
  expect(done.textContent).toContain('Child session: review');
  expect([...done.querySelectorAll('button')].some(button => button.textContent === 'Start' || button.textContent === 'Dismiss')).toBe(false);
});

test('open and answered proposals show native model IDs without discovery', async () => {
  const fetches = stubFetch(() => Response.json([]));
  const {root, store, send} = setup([{id: 'p', kind: 'proposal', text: 'Task', input: {name: 'child', title: 'Task', settings: {harness: 'opencode', model: 'm', effort: 'low'}}}]);
  await act(async () => store.update(state => ({...state, workspaces: [testWorkspace({id: 'w', sessions: [busy]})]})));
  expect(find(root, '[title="OpenCode m, effort low"]').textContent).toContain('m');
  send([{id: 'p', kind: 'proposal', text: 'Task', done: true, answer: 'child', input: {name: 'child', title: 'Task', settings: {harness: 'opencode', model: 'm', effort: 'low'}}}]);
  expect(find(root, '[title="OpenCode m, effort low"]').textContent).toContain('m');
  expect(fetches).not.toHaveBeenCalled();
});

test('a refused proposal action can be retried or dismissed', async () => {
  const fetches = spyOn(globalThis, 'fetch').mockResolvedValue(new Response('Could not start', {status: 400}));
  const {root} = setup([{id: 'p', kind: 'proposal', text: 'Task', input: {name: 'child', title: 'Task', settings: {harness: 'codex', model: 'm', effort: 'low'}}}]);
  const card = find(root, '[aria-label="Proposed child session"]');
  await act(async () => find(card, 'button').click());
  expect(find(card, 'button').hasAttribute('disabled')).toBe(false);
  fetches.mockResolvedValue(new Response(''));
  await act(async () => findAll(card, 'button')[1].click());
  expect(requestParams(fetches.mock.calls, 1).get('action')).toBe('dismiss-proposal');
});

test('an archived proposal cannot be started or dismissed', () => {
  const fetches = spyOn(globalThis, 'fetch');
  const {root} = setup([{id: 'p', kind: 'proposal', text: 'Task', input: {name: 'child', title: 'Task', settings: {harness: 'codex', model: 'm', effort: 'low'}}}], testSession({id: 's', ended: true, activity: 'exited'}));
  const buttons = findAll(root, '[aria-label="Proposed child session"] button');
  expect(buttons.map(button => button.textContent)).toEqual(['Start', 'Dismiss']);
  expect(buttons.every(button => button.hasAttribute('disabled'))).toBe(true);
  act(() => buttons.forEach(button => button.click()));
  expect(fetches).not.toHaveBeenCalled();
});

test('items replace themselves in place, and unchanged items are left alone', () => {
  const {root, send} = setup([{id: '1', kind: 'user', text: 'hi', seq: 1}, {id: '2', kind: 'text', text: 'Hel', seq: 2}]);
  const list = find(root, '[aria-label="Transcript"]');
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
  act(() => toggle(root, '<b>tool</b>').click());
  expect(root.querySelectorAll('b, script, img, u, i, a').length).toBe(0);
  for (const text of ['<b>tool</b>', '<img src=x onerror=1>', '<u>bad</u>', '<i>thinking</i>', '<a href=x>me</a>']) expect(root.textContent).toContain(text);
});

test('an expanded running tool explains missing details and updates as they arrive', () => {
  const {root, send} = setup([{id: 'c', kind: 'tool', tool: 'bash', seq: 1}]);
  act(() => toggle(root, 'Run command').click());
  expect(root.textContent).toContain('Waiting for tool details…');

  send([{id: 'c', kind: 'tool', tool: 'bash', input: {command: 'pwd'}, seq: 1}]);
  expect(root.textContent).toContain('Arguments{\n  "command": "pwd"\n}');
  expect(root.textContent).not.toContain('Waiting for tool details…');

  send([{id: 'c', kind: 'tool', tool: 'bash', input: {command: 'pwd'}, output: '/workspace', done: true, seq: 1}]);
  expect(root.textContent).toContain('Output/workspace');
});

test('an expanded completed tool with no details says so', () => {
  const {root} = setup([{id: 'c', kind: 'tool', tool: 'bash', done: true}]);
  act(() => toggle(root, 'Run command').click());
  expect(root.textContent).toContain('No details recorded.');
});

test('an attention request offers its choices, waits while sending, and opens again when refused', async () => {
  const refusal = Promise.withResolvers<Response>();
  const fetches = spyOn(globalThis, 'fetch').mockReturnValue(refusal.promise);
  const {root, send} = setup([{id: 'q', kind: 'attention', text: 'Proceed?', choices: ['yes', 'no']}]);
  const rows = () => findAll(root, '[role="radio"]');
  const list = () => find(root, '[role="radiogroup"]');
  expect(list().getAttribute('aria-label')).toBe('Choices');
  expect(findAll(list(), 'label').map(label => label.textContent)).toEqual(['yes', 'no']);
  expect(root.querySelector('input')).toBeNull();
  expect(submitButton(root).hasAttribute('disabled')).toBe(true);
  choose(root, 'yes');
  expect(fetches).not.toHaveBeenCalled();
  submit(root);
  expect(Object.fromEntries(requestParams(fetches.mock.calls, 0))).toEqual({action: 'answer', id: 's', request: 'q', choice: 'yes'});
  expect(rows().every(row => row.hasAttribute('disabled'))).toBe(true);
  choose(root, 'no');
  submit(root);
  expect(fetches).toHaveBeenCalledTimes(1);
  await act(async () => {
    refusal.resolve(new Response('no longer open', {status: 409}));
    await new Promise(resolve => setTimeout(resolve));
  });
  expect(rows().every(row => !row.hasAttribute('disabled'))).toBe(true);

  send([{id: 'q', kind: 'attention', text: 'Proceed?', done: true, answer: 'no', seq: 1}]);
  expect(rows()).toEqual([]);
  const answered = toggle(root, 'Answered');
  expect(answered.textContent).toBe('Answered no');
  expect(root.textContent).not.toContain('Question');
  act(() => answered.click());
  expect(root.textContent).toContain('QuestionProceed?Answerno');
});

test.each(['New session', 'Do it here'])('a task offer sends %s immediately, prevents duplicate answers, and records it', choice => {
  const fetches = spyOn(globalThis, 'fetch').mockReturnValue(new Promise<Response>(() => {}));
  const offer = {id: 'o', kind: 'attention' as const, offer: true, text: 'Fix the parser', choices: ['New session', 'Do it here']};
  const {root, send} = setup([offer]);
  const pending = pendingArea(root)!;
  expect(pending.querySelector('[role="radio"]')).toBeNull();
  const buttons = findAll(pending, 'button');
  expect(buttons.map(button => button.getAttribute('aria-label') ?? button.textContent)).toEqual(['Dismiss task', 'New session', 'Do it here']);
  act(() => buttons.find(button => button.textContent === choice)!.click());
  expect(buttons.every(button => button.hasAttribute('disabled'))).toBe(true);
  act(() => buttons.forEach(button => button.click()));
  expect(fetches).toHaveBeenCalledTimes(1);
  expect(Object.fromEntries(requestParams(fetches.mock.calls, 0))).toEqual({action: 'answer', id: 's', request: 'o', choice});
  send([{...offer, done: true, answer: choice}]);
  expect(pendingArea(root)).toBeNull();
  expect(toggle(root, 'Answered').textContent).toBe('Answered ' + choice);
});

test('current and saved two-choice offers can be dismissed and leave the pending area', () => {
  const fetches = spyOn(globalThis, 'fetch').mockReturnValue(new Promise<Response>(() => {}));
  for (const choices of [['New session', 'Do it here', 'Dismiss'], ['New session', 'Do it here']]) {
    const offer = {id: 'o', kind: 'attention' as const, offer: true, text: 'Fix the parser', choices};
    const {root, send} = setup([offer]);
    act(() => button(pendingArea(root)!, 'Dismiss task').click());
    const call = fetches.mock.calls.at(-1)!;
    expect(Object.fromEntries(new URL(String(call[0]), 'http://x/').searchParams)).toEqual({action: 'answer', id: 's', request: 'o', choice: 'Dismiss'});
    send([{...offer, done: true, answer: 'Dismiss'}]);
    expect(pendingArea(root)).toBeNull();
    expect(toggle(root, 'Answered').textContent).toBe('Answered Dismiss');
  }
  expect(fetches).toHaveBeenCalledTimes(2);
});

test('an offer in an archived transcript cannot be answered', () => {
  const fetches = spyOn(globalThis, 'fetch');
  const {root} = setup([{id: 'o', kind: 'attention', offer: true, text: 'Fix the parser', choices: ['New session', 'Do it here']}], testSession({id: 's', ended: true, activity: 'exited'}));
  const buttons = findAll(pendingArea(root)!, 'button');
  expect(buttons).toHaveLength(3);
  expect(buttons.every(button => button.hasAttribute('disabled'))).toBe(true);
  act(() => buttons.forEach(button => button.click()));
  expect(fetches).not.toHaveBeenCalled();
});

test('a refused task placement can be retried', async () => {
  const fetches = stubFetch(() => new Response('Could not answer', {status: 409}));
  const {root} = setup([{id: 'o', kind: 'attention', offer: true, text: 'Fix the parser', choices: ['New session', 'Do it here']}]);
  const buttons = findAll(pendingArea(root)!, 'button');
  await act(async () => buttons[2].click());
  expect(buttons.every(button => !button.hasAttribute('disabled'))).toBe(true);
  await act(async () => buttons[2].click());
  expect(fetches).toHaveBeenCalledTimes(2);
});

test('an approval request offers to always allow its tool in either scope', async () => {
  const fetches = spyOn(globalThis, 'fetch').mockResolvedValue(new Response(''));
  for (const [i, scope] of ['session', 'all'].entries()) {
    const {root} = setup([{id: 'q', kind: 'attention', text: 'Allow Bash?', choices: ['Allow', 'Deny'], grant: 'Bash'}]);
    const buttons = findAll(find(root, '[aria-label="Always allow"]'), 'button');
    expect(buttons.map(b => b.textContent)).toEqual(['Always allow in this session', 'Always allow in all sessions']);
    await act(async () => { clickControl(buttons[i]); });
    expect(Object.fromEntries(requestParams(fetches.mock.calls, i))).toEqual({action: 'allow', id: 's', request: 'q', scope});
  }
});

test('a typed answer is sent with Enter and not when empty', async () => {
  const fetches = spyOn(globalThis, 'fetch').mockResolvedValue(new Response(''));
  const {root} = setup([{id: 'q', kind: 'attention', text: 'Name?', free: true, choices: ['Ada']}]);
  const field = element(root, 'input', HTMLInputElement);
  expect(field.getAttribute('aria-label')).toBe('Other response');
  expect(submitButton(root).hasAttribute('disabled')).toBe(true);
  act(() => clickControl(submitButton(root)));
  act(() => field.dispatchEvent(new KeyboardEvent('keydown', {key: 'Enter', bubbles: true})));
  expect(fetches).not.toHaveBeenCalled();
  const otherRadio = find(root, '[role="radio"][aria-label="Other"]');
  act(() => clickControl(otherRadio));
  expect(otherRadio.getAttribute('aria-checked')).toBe('true');
  expect(submitButton(root).hasAttribute('disabled')).toBe(true);
  choose(root, 'Ada');
  expect(otherRadio.getAttribute('aria-checked')).toBe('false');
  setValue(field, 'Grace');
  act(() => field.dispatchEvent(new Event('input', {bubbles: true})));
  expect(labelled(root, 'Ada').getAttribute('aria-checked')).toBe('false');
  expect(submitButton(root).hasAttribute('disabled')).toBe(false);
  await act(async () => field.dispatchEvent(new KeyboardEvent('keydown', {key: 'Enter', bubbles: true})));
  expect(requestParams(fetches.mock.calls, 0).get('choice')).toBe('');
  expect(fetches.mock.calls[0][1]?.body).toBe('Grace');
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
  const worked = toggle(root, 'Worked for');
  expect(worked.getAttribute('aria-expanded')).toBe('false');
  const pending = pendingArea(root)!;
  expect(labelled(pending, 'ok')).toBeDefined();
  expect(find(root, '[aria-label="Transcript"]').contains(pending)).toBe(false);
  expect(root.textContent).not.toContain('Answered');
  send([{id: 'q', parent: 'a', kind: 'attention', text: 'Allow?', done: true, answer: 'ok', at: 1300}]);
  expect(pendingArea(root)).toBeNull();
  act(() => worked.click());
  act(() => toggle(root, 'Run agent').click());
  expect(toggle(root, 'Answered').textContent).toBe('Answered ok');
});

test('pending questions and their draft stay outside the history list while output arrives', () => {
  const {root, send} = setup([
    {id: 'q1', kind: 'attention', text: 'First?', free: true},
    {id: 'q2', kind: 'attention', text: 'Second?', choices: ['yes']},
  ]);
  const pending = pendingArea(root)!;
  const field = element(pending, 'input', HTMLInputElement);
  setValue(field, 'My answer');
  act(() => field.dispatchEvent(new Event('input', {bubbles: true})));
  const view = find(root, '[aria-label="Transcript"]').parentElement!;
  view.scrollTop = 0;
  act(() => view.dispatchEvent(new Event('scroll')));
  send([{id: 'output', kind: 'text', text: 'More work'}]);
  expect(pendingArea(root) === pending).toBe(true);
  expect(find(pending, 'input') === field).toBe(true);
  expect(field.value).toBe('My answer');
  expect(pending.textContent).toContain('First?');
  expect(pending.textContent).not.toContain('Second?');
  expect(pending.textContent).toContain('Question 1 of 2');
  expect(find(view, '[aria-label="Transcript"]').textContent).toBe('More work');
  send([{id: 'q1', kind: 'attention', text: 'First?', done: true, answer: 'My answer', seq: 0}]);
  expect(toggle(view, 'Answered').textContent).toBe('Answered My answer');
  expect(pending.textContent).toContain('Second?');
  expect(pending.textContent).not.toContain('Question 1 of');
});

test('a turn draws as it runs and folds its activity under Worked for once it finishes', () => {
  const {root, send} = setup([
    {id: 'u', kind: 'user', text: 'Go', at: 1000},
    marker('w', 'working', 1100),
    {id: 'c', kind: 'tool', tool: 'bash', at: 1200},
    {id: 't', kind: 'text', text: 'Done', at: 1400},
  ]);
  const list = find(root, '[aria-label="Transcript"]');
  expect(root.textContent).not.toContain('Worked');
  expect(list.children[1].textContent).toBe('Run command');
  send([marker('i', 'idle', 2000, ['t'])]);
  expect(list.children[1].querySelector('button')!.textContent).toBe('Worked for 1s');
  expect(root.textContent).not.toContain('Run command');
  expect(list.children[2].textContent).toBe('Done');
});

const working = (root: ParentNode) => findAll(root, '[role=status]').find(row => row.textContent === 'Working…');
const wait = (ms: number) => act(() => jest.advanceTimersByTime(ms));

test('a running turn that sends nothing for a while ends the list with a working row until output arrives', () => {
  jest.useFakeTimers();
  const {root, send} = setup([
    {id: 'u', kind: 'user', text: 'Go', at: 1000},
    marker('w', 'working', 1100),
  ]);
  wait(quietDelay - 1);
  expect(working(root)).toBeUndefined();
  wait(1);
  expect(find(root, '[aria-label="Transcript"]').lastElementChild).toBe(working(root)!);
  send([{id: 't', kind: 'text', text: 'Part', at: 1200}]);
  expect(working(root)).toBeUndefined();
  wait(quietDelay - 1);
  send([{id: 't', kind: 'text', text: 'Partial answer', at: 1200, seq: 3}]);
  wait(quietDelay - 1);
  expect(working(root)).toBeUndefined();
  wait(1);
  expect(working(root)).toBeDefined();
});

test('a running tool, an open question, or an idle session shows no working row', () => {
  jest.useFakeTimers();
  const start: NewItem[] = [{id: 'u', kind: 'user', text: 'Go', at: 1000}, marker('w', 'working', 1100)];
  const tool = setup([...start, {id: 'c', kind: 'tool', tool: 'bash', at: 1200}]);
  const question = setup([...start, {id: 'q', kind: 'attention', text: 'Allow?', choices: ['ok'], at: 1200}]);
  const idle = setup(start, testSession({id: 's', activity: 'idle'}));
  wait(quietDelay);
  for (const {root} of [tool, question, idle]) expect(working(root)).toBeUndefined();
  tool.send([{id: 'c', kind: 'tool', tool: 'bash', at: 1200, done: true, seq: 3}]);
  wait(quietDelay);
  expect(working(tool.root)).toBeDefined();
});

test('an open proposal, which does not stop the agent, still shows the working row', () => {
  jest.useFakeTimers();
  const {root} = setup([
    {id: 'u', kind: 'user', text: 'Go', at: 1000},
    marker('w', 'working', 1100),
    {id: 'p', kind: 'proposal', text: 'Later', input: {name: 'later', title: 'Later', settings: {harness: 'codex', model: 'm', effort: 'low'}}},
  ]);
  wait(quietDelay);
  expect(working(root)).toBeDefined();
});

test('short reasoning shows in full without a heading', () => {
  const {root} = setup([
    {id: 'u', kind: 'user', text: 'Go'},
    {id: 'r1', kind: 'reasoning', text: 'First idea'},
    {id: 'r2', kind: 'reasoning', text: 'Second'},
  ]);
  expect(findAll(root, 'button[aria-expanded]')).toHaveLength(0);
  expect(find(root, '[aria-label="Transcript"]').textContent).toBe('GoFirst ideaSecond');
});

test('long reasoning shows its last lines under Thinking, opens to its whole trace while it streams, and reads Thought once done', () => {
  const {root, send} = setup([
    {id: 'u', kind: 'user', text: 'Go'},
    {id: 'r1', kind: 'reasoning', text: 'One\nTwo'},
    {id: 'r2', kind: 'reasoning', text: 'Three\n\nFour'},
  ]);
  const thinking = toggle(root, 'Thinking…');
  expect(thinking.getAttribute('aria-expanded')).toBe('false');
  expect(find(root, '[aria-label="Transcript"]').textContent).toBe('GoThinking…TwoThreeFour');
  act(() => thinking.click());
  expect(root.textContent).toContain('One\nTwo');
  send([{id: 'r2', kind: 'reasoning', text: 'Three\n\nFour\nFive', seq: 3}]);
  expect(root.textContent).toContain('Four\nFive');
  send([{id: 'r1', kind: 'reasoning', text: 'One\nTwo', done: true, seq: 2}, {id: 'r2', kind: 'reasoning', text: 'Three\n\nFour\nFive', done: true, seq: 3}]);
  expect(thinking.textContent).toBe('Thought');
  act(() => thinking.click());
  expect(find(root, '[aria-label="Transcript"]').textContent).toBe('GoThoughtThreeFourFive');
});

test('a long reasoning line puts the run under Thinking, one line on screen per line', () => {
  const line = 'x'.repeat(lineLength + 1);
  const {root} = setup([{id: 'r', kind: 'reasoning', text: 'Short\n' + line}]);
  expect(toggle(root, 'Thinking…')).toBeDefined();
  const shown = findAll(root, 'p.truncate');
  expect(shown.map(p => p.textContent)).toEqual(['Short', line]);
});

test('empty reasoning and text do not create an empty activity disclosure', () => {
  const {root, send} = setup([
    {id: 'u', kind: 'user', text: 'Go', at: 1000},
    marker('w', 'working', 1100),
    {id: 'r', kind: 'reasoning', at: 1200},
    {id: 'blank', kind: 'text', text: '  \n', at: 1300},
    {id: 't', kind: 'text', text: 'Done', at: 1400},
    marker('i', 'idle', 5000, ['t']),
  ]);
  expect(root.textContent).not.toContain('Worked');
  expect(find(root, '[aria-label="Transcript"]').textContent).toBe('GoDone');

  send([{id: 'r', kind: 'reasoning', text: 'Checking the answer', at: 1200, seq: 3}]);
  act(() => toggle(root, 'Worked for').click());
  expect(root.textContent).toContain('Checking the answer');
});

test('image-only activity remains available to expand', () => {
  const {root} = setup([
    marker('w', 'working', 1000),
    {id: 'image', kind: 'text', images: ['/transcript-image?index=0'], at: 1200},
    {id: 't', kind: 'text', text: 'Done', at: 1400},
    marker('i', 'idle', 5000, ['t']),
  ]);
  act(() => toggle(root, 'Worked for').click());
  expect(find(root, 'img').getAttribute('src')).toBe('/transcript-image?index=0');
});

// Happy DOM does not perform layout. These assertions protect the scrolling
// boundary and tab order that keep long pending forms above the composer.
test('pending forms have a shrinking scroll region before the composer', () => {
  const items: Item[] = Array.from({length: 6}, (_, i) => ({id: `q${i}`, kind: 'attention', text: `Question ${i}`, choices: ['Yes', 'No'], free: true}));
  items.push({id: 'p', kind: 'proposal', text: 'Long task', input: {name: 'child', title: 'Long task', settings: {harness: 'codex', model: 'm', effort: 'low'}, message: 'A long brief.\n'.repeat(40)}});
  const store = createStore(testState({session: emptySession('s')}));
  store.update(s => receivedTranscript(s, {type: 'transcript', session: 's', reset: true, items: items.map((item, seq) => ({...item, seq}))}));
  const root = mount(store, <Transcript session={busy} footer={<textarea aria-label="Message" />} />);
  const pending = pendingArea(root)!;
  const content = pending.parentElement!, footer = content.parentElement!;
  expect(footer.classList.contains('min-h-0')).toBe(true);
  expect(footer.classList.contains('max-h-3/4')).toBe(true);
  expect(content.classList.contains('overflow-y-auto')).toBe(true);
  expect(pending.classList.contains('min-h-24')).toBe(true);
  expect(pending.classList.contains('overflow-y-auto')).toBe(true);
  expect(pending.textContent).toContain('Question 0YesNoQuestion 1 of 6');
  expect(findAll(pending, 'button').filter(b => b.textContent === 'Next')).toHaveLength(1);
  expect(findAll(pending, '[aria-label="Proposed child session"] button').map(button => button.textContent)).toEqual(['Start', 'Dismiss']);
  const controls = findAll(footer, 'button, input, textarea').filter(control => !control.hasAttribute('tabindex') || control.tabIndex >= 0);
  expect(controls.at(-1)?.getAttribute('aria-label')).toBe('Message');
});

test('incoming, outgoing, and automatic messages collapse to one line and can be reopened', () => {
  const image = '/transcript-image?index=0';
  const {root} = setup([
    {id: 'user', kind: 'user', text: 'A regular user prompt'},
    {id: 'incoming', kind: 'user', from: 'child', text: 'First line\nSecond line', images: [image]},
    {id: 'outgoing', kind: 'tool', server: 'hypercube', tool: 'send_message', input: {to: 'child', text: 'A single line message'}, done: true},
    {id: 'failed', kind: 'tool', server: 'hypercube', tool: 'send_message', input: {to: 'child', text: 'Try again'}, failed: true, done: true, output: 'Delivery failed', images: [image]},
    {id: 'automatic', kind: 'user', automatic: true, text: 'A report\nFull details', images: [image]},
  ]);
  const list = find(root, '[aria-label="Transcript"]');
  expect(list.children[0].textContent).toBe('A regular user prompt');
  expect(list.children[0].querySelector('button[aria-expanded]')).toBeNull();
  const triggers = findAll(root, 'button[aria-expanded]');
  expect(triggers.map(trigger => trigger.textContent)).toEqual(['From childFirst line', 'To childA single line message', 'To childTry again', 'Automatic messageA report']);
  expect(root.textContent).not.toContain('Child reports');
  for (const [i, trigger] of triggers.entries()) {
    const message = trigger.parentElement!;
    expect(trigger.getAttribute('aria-expanded')).toBe('false');
    expect(message.querySelector('img')).toBeNull();
    act(() => trigger.click());
    expect(trigger.getAttribute('aria-expanded')).toBe('true');
    expect(trigger.textContent).not.toContain(['First line', 'A single line message', 'Try again', 'A report'][i]);
    if (i === 0) expect(message.textContent).toContain('Second line');
    if (i === 2) expect(message.textContent).toContain('Delivery failed');
    if (i !== 1) expect(message.querySelector('img')).not.toBeNull();
    act(() => trigger.click());
    expect(trigger.getAttribute('aria-expanded')).toBe('false');
    expect(message.textContent).toBe(trigger.textContent);
  }
});
