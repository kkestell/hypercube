import {afterEach, expect, jest, spyOn, test} from 'bun:test';
import {act} from 'preact/test-utils';
import {mount, unmountAll} from '../../test-render.jsx';
import {emptySession} from '../sessions/selection.js';
import {createStore} from '../store.js';
import {Transcript} from './transcript.jsx';
import {receivedTranscript} from './transcripts.js';

let spies = [];
afterEach(() => {
  for (const s of spies) s.mockRestore();
  spies = [];
  unmountAll();
  jest.useRealTimers();
});

const busy = {id: 's', running: true, activity: 'busy'};

function setup(items, session = busy) {
  const store = createStore({workspaces: [], drafts: {}, errors: [], session: emptySession('s')});
  const send = (items, reset = false) => store.update(s => receivedTranscript(s, {session: 's', reset, items}));
  send(withSeq(items), true);
  const root = mount(store, <Transcript session={session} />);
  return {store, root, send: items => send(withSeq(items))};
}
let seq = 0;
const withSeq = items => items.map(item => ({seq: item.seq ?? ++seq, ...item}));

test('items replace themselves in place, and unchanged items are left alone', () => {
  const {root, send} = setup([{id: '1', kind: 'user', text: 'hi', seq: 1}, {id: '2', kind: 'text', text: 'Hel', seq: 2}]);
  const list = root.querySelector('.transcript-list');
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
  act(() => root.querySelector('.reasoning-item summary').click());
  expect(root.querySelectorAll('b, script, img, u, i, a').length).toBe(0);
  for (const text of ['<b>tool</b>', '<img src=x onerror=1>', '<u>bad</u>', '<i>thinking</i>', '<a href=x>me</a>']) expect(root.textContent).toContain(text);
});

test('a tool call shows its title, status, input, output, and nested items', () => {
  const {root, send} = setup([{id: 'c', kind: 'tool', tool: 'bash', input: {command: 'ls'}}]);
  const call = () => root.querySelector('.tool-items-call');
  expect(call().querySelector('summary').textContent).toContain('bash ls');
  expect(call().querySelector('[title="Running"]')).not.toBeNull();
  expect(call().querySelector('pre').textContent).toBe('{\n  "command": "ls"\n}');
  send([{id: 'n', parent: 'c', kind: 'text', text: 'inside'}, {id: 'c', kind: 'tool', tool: 'bash', input: {command: 'ls'}, output: 'a.txt', done: true, failed: true}]);
  expect(call().querySelector('[title="Running"]')).toBeNull();
  expect(call().querySelector('[title="Failed"]')).not.toBeNull();
  expect(call().querySelectorAll('pre')[1].textContent).toBe('a.txt');
  expect(call().querySelector('.tool-items-children').textContent).toBe('inside');
});

test.each([['the user', undefined], ['another session', 'child']])('a tool run passes over a message from %s and counts how long it works until other output', (_source, from) => {
  jest.useFakeTimers();
  jest.setSystemTime(6000);
  const first = {id: 'a', kind: 'tool', tool: 'read', input: {path: '/first'}, at: 1000, seq: 1};
  const {root, send} = setup([first]);
  const list = root.querySelector('.transcript-list');
  const group = list.firstElementChild;
  const summary = group.querySelector('summary');
  const chevron = summary.querySelector('.disclosure-chevron');
  expect(chevron).not.toBeNull();
  expect(summary.querySelector('.disclosure-heading').lastElementChild).toBe(chevron);
  expect(summary.querySelector('[title="Running"]')).toBeNull();
  expect(summary.textContent).toBe('Working for 5s…');
  expect(summary.querySelector('.disclosure-preview')).toBeNull();
  act(() => jest.advanceTimersByTime(1000));
  expect(summary.textContent).toBe('Working for 6s…');
  act(() => summary.click());
  expect(group.open).toBe(true);
  expect(summary.textContent).toBe('Working for 6s…');
  expect(group.querySelector('.tool-items-call [title="Running"]')).not.toBeNull();

  send([{...first, done: true}, {id: 'u', kind: 'user', from, text: 'Keep going', at: 3000, seq: 2}]);
  expect(list.children[1].textContent).toContain('Keep going');
  expect(summary.textContent).toBe('Working for 6s…');

  send([{id: 'b', kind: 'tool', tool: 'bash', input: {command: 'x'}, at: 5000, seq: 3}]);
  expect(list.children.length).toBe(2);
  expect(group.querySelectorAll('.tool-items-call').length).toBe(2);
  expect(group.querySelectorAll('.tool-items-call [title="Running"]').length).toBe(1);

  act(() => summary.click());
  expect(group.open).toBe(false);
  expect(summary.textContent).toBe('Working for 6s…');

  send([{id: 'b', kind: 'tool', tool: 'bash', input: {command: 'x'}, at: 5000, done: true, end: 9000, seq: 3}, {id: 't', kind: 'text', text: 'Finished', at: 10000, seq: 4}]);
  expect(list.firstElementChild).toBe(group);
  expect(summary.textContent).toBe('Worked for 9s');
  expect(summary.querySelector('.disclosure-heading').lastElementChild).toBe(chevron);
  act(() => jest.advanceTimersByTime(5000));
  expect(summary.textContent).toBe('Worked for 9s');
});

const choose = (root, choice) => {
  act(() => root.querySelector(`vscode-button[data-choice="${choice}"]`).click());
};

test('an attention request offers its choices, waits while sending, and opens again when refused', async () => {
  let answer;
  const fetches = spyOn(globalThis, 'fetch').mockImplementation(() => new Promise(resolve => { answer = resolve; }));
  spies.push(fetches);
  const {root, send} = setup([{id: 'q', kind: 'attention', text: 'Proceed?', choices: ['yes', 'no']}]);
  const rows = () => [...root.querySelectorAll('vscode-button[data-choice]')];
  const list = () => root.querySelector('[role="group"]');
  expect(list().getAttribute('aria-label')).toBe('Choices');
  expect(rows().map(r => r.textContent)).toEqual(['yes', 'no']);
  expect(root.querySelector('vscode-textfield')).toBeNull();
  choose(root, 'yes');
  const url = new URL(fetches.mock.calls[0][0], 'http://x/');
  expect(Object.fromEntries(url.searchParams)).toEqual({action: 'answer', id: 's', request: 'q', choice: 'yes'});
  expect(rows().every(row => row.hasAttribute('disabled'))).toBe(true);
  choose(root, 'no');
  expect(fetches).toHaveBeenCalledTimes(1);
  await act(async () => {
    answer(new Response('no longer open', {status: 409}));
    await new Promise(resolve => setTimeout(resolve));
  });
  expect(rows().every(row => !row.hasAttribute('disabled'))).toBe(true);

  send([{id: 'q', kind: 'attention', text: 'Proceed?', done: true, answer: 'no', seq: 1}]);
  expect(rows()).toEqual([]);
  expect(root.querySelector('.attention-item-answered').textContent).toBe('Proceed?');
  expect(root.querySelector('.attention-item-answer').textContent).toBe('no');
});

test('a choice shows its associated note', () => {
  const {root} = setup([{id: 'q', kind: 'attention', text: 'Language?', choices: ['Python', 'Go'], notes: ['Readable', '']}]);
  const [python, go] = root.querySelectorAll('vscode-button[data-choice]');
  expect(python.textContent).toBe('Python');
  expect(document.getElementById(python.getAttribute('aria-describedby')).textContent).toBe('Readable');
  expect(go.hasAttribute('aria-describedby')).toBe(false);
});

test('a typed answer is sent with Enter and not when empty', async () => {
  const fetches = spyOn(globalThis, 'fetch').mockImplementation(async () => new Response(''));
  spies.push(fetches);
  const {root} = setup([{id: 'q', kind: 'attention', text: 'Name?', free: true, choices: ['Ada']}]);
  const field = root.querySelector('vscode-textfield');
  const sendButton = root.querySelector('.attention-form-entry vscode-button');
  const label = root.querySelector('vscode-label');
  expect(label.textContent).toBe('Other response');
  expect(label.getAttribute('for')).toBe(field.id);
  expect(field.querySelector('vscode-button')).toBeNull();
  expect(sendButton.textContent).toBe('Send');
  expect(sendButton.hasAttribute('disabled')).toBe(true);
  act(() => sendButton.click());
  expect(fetches).not.toHaveBeenCalled();
  act(() => field.dispatchEvent(new KeyboardEvent('keydown', {key: 'Enter'})));
  expect(fetches).not.toHaveBeenCalled();
  field.value = 'Ada';
  act(() => field.dispatchEvent(new Event('input')));
  expect(sendButton.hasAttribute('disabled')).toBe(false);
  await act(async () => field.dispatchEvent(new KeyboardEvent('keydown', {key: 'Enter'})));
  const [url, options] = fetches.mock.calls[0];
  expect(new URL(url, 'http://x/').searchParams.get('choice')).toBe('');
  expect(options.body).toBe('Ada');
});

test('the Send button beside the field sends the typed answer', async () => {
  const fetches = spyOn(globalThis, 'fetch').mockImplementation(async () => new Response(''));
  spies.push(fetches);
  const {root} = setup([{id: 'q', kind: 'attention', text: 'Name?', free: true}]);
  const field = root.querySelector('vscode-textfield');
  expect(root.querySelector('vscode-label').textContent).toBe('Your response');
  field.value = 'Grace';
  act(() => field.dispatchEvent(new Event('input')));
  await act(async () => root.querySelector('.attention-form-entry vscode-button').click());
  expect(fetches.mock.calls[0][1].body).toBe('Grace');
});

test('a request inside collapsed tool calls opens them', () => {
  const {root} = setup([
    {id: 'a', kind: 'tool', tool: 'agent'},
    {id: 'b', parent: 'a', kind: 'tool', tool: 'inner'},
    {id: 'q', parent: 'b', kind: 'attention', text: 'Allow?', choices: ['ok']},
  ]);
  const details = [...root.querySelectorAll('details')];
  expect(details.length).toBe(4);
  expect(details.every(d => d.open)).toBe(true);
});

test('the list follows its end until the user scrolls up', () => {
  const {root, send} = setup([{id: '1', kind: 'text', text: 'a'}]);
  const view = root.querySelector('.transcript');
  let height = 1000;
  Object.defineProperty(view, 'scrollHeight', {get: () => height});
  Object.defineProperty(view, 'clientHeight', {get: () => 300});
  const scroll = top => {
    view.scrollTop = top;
    view.dispatchEvent(new Event('scroll'));
  };
  send([{id: '2', kind: 'text', text: 'b'}]);
  expect(view.scrollTop).toBe(1000);
  scroll(500);
  height = 1200;
  send([{id: '3', kind: 'text', text: 'c'}]);
  expect(view.scrollTop).toBe(500);
  scroll(899);
  send([{id: '4', kind: 'text', text: 'd'}]);
  expect(view.scrollTop).toBe(1200);
});

const longText = Array.from({length: 10}, (_, i) => 'line ' + i).join('\n');

test('agent messages show the sender and first line, and expand to the full message', () => {
  const {root, send} = setup([
    {id: 'in', kind: 'user', from: 'c1', text: longText, done: true},
    {id: 'out', kind: 'tool', server: 'hypercube', tool: 'send_message', input: {to: 'c1', text: longText}},
    {id: 'own', kind: 'user', text: longText},
  ]);
  const [incoming, outgoing, own] = root.querySelectorAll('.message-item');
  expect(incoming.querySelector('.disclosure-heading').textContent).toBe('From c1');
  expect(outgoing.querySelector('.disclosure-heading').textContent).toBe('To c1');
  expect(outgoing.querySelector('[title="Running"]')).not.toBeNull();
  for (const message of [incoming, outgoing]) {
    expect(message.querySelector('.message-item-bubble')).toBeNull();
    expect(message.querySelector('.disclosure-preview').textContent).toBe('line 0');
    expect(message.querySelector('.message-item-text')).toBeNull();
    act(() => message.querySelector('summary').click());
    expect(message.open).toBe(true);
    expect(message.querySelector('.disclosure-preview')).toBeNull();
    expect(message.querySelector('.message-item-text').textContent).toBe(longText);
  }
  send([{id: 'out', kind: 'tool', server: 'hypercube', tool: 'send_message', input: {to: 'c1', text: longText}, done: true, failed: true, output: 'Delivery failed'}]);
  expect(outgoing.open).toBe(true);
  expect(outgoing.querySelector('[title="Running"]')).toBeNull();
  expect(outgoing.querySelector('[title="Failed"]')).not.toBeNull();
  expect(outgoing.querySelector('.message-item-body .message-item-failure').textContent).toBe('Delivery failed');
  act(() => incoming.querySelector('summary').click());
  expect(incoming.open).toBe(false);
  expect(incoming.querySelector('.disclosure-preview').textContent).toBe('line 0');
  expect(own.querySelector('.message-item-bubble')).not.toBeNull();
  expect(own.querySelector('.message-item-text').textContent).toBe(longText);
  expect(own.querySelector('summary')).toBeNull();
});

test('short agent messages also expand, including their attached images', () => {
  const {root} = setup([{id: 'in', kind: 'user', from: 'c1', text: 'One line', images: [{mime_type: 'image/png', data: 'iVBORw0KGgo='}]}]);
  const message = root.querySelector('.message-item-agent');
  expect(message.querySelector('.disclosure-preview').textContent).toBe('One line');
  expect(message.querySelector('img')).toBeNull();
  act(() => message.querySelector('summary').click());
  expect(message.querySelector('.message-item-text').textContent).toBe('One line');
  expect(message.querySelector('img').getAttribute('src')).toBe('data:image/png;base64,iVBORw0KGgo=');
});

test('Thinking counts its time while it streams and shows only a header until expanded', () => {
  jest.useFakeTimers();
  jest.setSystemTime(6000);
  const {root, send} = setup([{id: 't', kind: 'reasoning', text: longText, at: 1000, seq: 1}]);
  const item = root.querySelector('.reasoning-item');
  const summary = item.querySelector('summary');
  expect(summary.textContent).toBe('Thinking for 5s…');
  expect(item.querySelector('.reasoning-item-text')).toBeNull();
  expect(item.textContent).toBe('Thinking for 5s…');
  act(() => jest.advanceTimersByTime(147000));
  expect(summary.textContent).toBe('Thinking for 2m 32s…');
  act(() => summary.click());
  expect(item.querySelector('.reasoning-item-text').textContent).toBe(longText);
  send([{id: 't', kind: 'reasoning', text: longText + '\nmore', at: 1000, seq: 1}]);
  expect(item.querySelector('details').open).toBe(true);
  expect(item.querySelector('.reasoning-item-text').textContent).toEndWith('more');
  send([{id: 't', kind: 'reasoning', text: longText, at: 1000, end: 191000, done: true, seq: 1}]);
  expect(summary.textContent).toBe('Thought for 3m 10s');
  act(() => jest.advanceTimersByTime(10000));
  expect(summary.textContent).toBe('Thought for 3m 10s');
});

test('finished Thinking shows its real duration whether or not anything follows it', () => {
  jest.useFakeTimers();
  jest.setSystemTime(500000);
  const {root, send} = setup([{id: 't', kind: 'reasoning', text: 'x', at: 1000, end: 4000, done: true, seq: 1}], {id: 's', running: false, activity: 'idle'});
  const summary = root.querySelector('.reasoning-item summary');
  expect(summary.textContent).toBe('Thought for 3s');
  send([{id: 'u', kind: 'reasoning', text: 'y', at: 10000, done: true, seq: 2}]);
  expect(root.querySelectorAll('.reasoning-item summary')[1].textContent).toBe('Thought');
});

test('Thinking saved without an end shows the time up to the next row', () => {
  const {root} = setup([
    {id: 't', kind: 'reasoning', text: 'x', at: 1000, done: true, seq: 1},
    {id: 'u', kind: 'user', text: 'hi', at: 2000, seq: 2},
    {id: 'x', kind: 'text', text: 'Answer', at: 4000, seq: 3},
  ], {id: 's', running: false, activity: 'idle'});
  expect(root.querySelector('.reasoning-item summary').textContent).toBe('Thought for 3s');
});

test('a message that arrives during Thinking leaves it counting until it finishes', () => {
  jest.useFakeTimers();
  jest.setSystemTime(3000);
  const thought = {id: 't', kind: 'reasoning', text: 'x', at: 1000, seq: 1};
  const {root, send} = setup([thought]);
  const summary = root.querySelector('.reasoning-item summary');
  send([{id: 'u', kind: 'user', text: 'hello', at: 2000, seq: 2}]);
  act(() => jest.advanceTimersByTime(8000));
  expect(summary.textContent).toBe('Thinking for 10s…');
  send([{...thought, done: true, end: 12000}]);
  expect(summary.textContent).toBe('Thought for 11s');
});

test('alternating thinking and tools stays in one working block and completed timers stop', () => {
  jest.useFakeTimers();
  jest.setSystemTime(6000);
  const {root, send} = setup([{id: 'r', kind: 'reasoning', text: 'Plan', at: 1000, seq: 1}]);
  const list = root.querySelector('.transcript-list');
  const run = list.firstElementChild;
  const summary = run.querySelector('summary');
  act(() => summary.click());
  const thinking = run.querySelector('.reasoning-item summary');
  expect(summary.textContent).toBe('Working for 5s…');
  send([
    {id: 'c', kind: 'tool', tool: 'read', at: 4000, end: 5000, done: true, seq: 2},
    {id: 's', kind: 'reasoning', text: 'Next step', at: 5000, seq: 3},
  ]);
  expect(list.children.length).toBe(1);
  expect(list.firstElementChild).toBe(run);
  expect(thinking.textContent).toBe('Thought for 3s');
  expect(run.querySelectorAll('.reasoning-item').length).toBe(2);
  act(() => jest.advanceTimersByTime(2000));
  expect(thinking.textContent).toBe('Thought for 3s');
  expect(summary.textContent).toBe('Working for 7s…');
  send([{id: 'a', kind: 'text', text: 'Answer', at: 8000, seq: 4}]);
  expect(summary.textContent).toBe('Worked for 7s');
  expect(run.querySelectorAll('.reasoning-item summary')[1].textContent).toBe('Thought for 3s');
  act(() => jest.advanceTimersByTime(60000));
  expect(summary.textContent).toBe('Worked for 7s');
  expect(thinking.textContent).toBe('Thought for 3s');
});

test('unfinished thinking in an idle session stops counting', () => {
  jest.useFakeTimers();
  jest.setSystemTime(6000);
  const {root} = setup([{id: 'r', kind: 'reasoning', text: 'Old thought', at: 1000}], {id: 's', running: true, activity: 'idle'});
  expect(root.querySelector('summary').textContent).toBe('Worked');
  const summary = root.querySelector('.reasoning-item summary');
  expect(summary.textContent).toBe('Thought');
  act(() => jest.advanceTimersByTime(60000));
  expect(summary.textContent).toBe('Thought');
});

test('a working block waits for its shell after the agent replies', () => {
  jest.useFakeTimers();
  jest.setSystemTime(6000);
  const tool = {id: 'a', kind: 'tool', tool: 'shell', at: 1000, seq: 1};
  const {root, send} = setup([tool], {id: 's', running: true, activity: 'idle'});
  const summary = root.querySelector('summary');
  send([{id: 't', kind: 'text', text: 'Preview ready', at: 4000, seq: 2}]);
  expect(summary.textContent).toBe('Working for 5s…');
  expect(root.querySelector('.tool-items-call [title="Running"]')).not.toBeNull();
  act(() => jest.advanceTimersByTime(3000));
  expect(summary.textContent).toBe('Working for 8s…');
  send([{...tool, done: true, end: 9000}]);
  expect(summary.textContent).toBe('Worked for 8s');
  expect(root.querySelector('.tool-items-call [title="Running"]')).toBeNull();
  act(() => jest.advanceTimersByTime(60000));
  expect(summary.textContent).toBe('Worked for 8s');
});

test('a finished last run in a session that is not working shows a fixed duration', () => {
  jest.useFakeTimers();
  jest.setSystemTime(900000);
  const {root} = setup([{id: 'a', kind: 'tool', tool: 'read', done: true, at: 1000, end: 5000, seq: 1}], {id: 's', running: true, activity: 'idle'});
  const summary = root.querySelector('summary');
  expect(summary.textContent).toBe('Worked for 4s');
  act(() => jest.advanceTimersByTime(60000));
  expect(summary.textContent).toBe('Worked for 4s');
});

test('expanded Thinking stays scrolled to the newest text until the user scrolls up', () => {
  const {root, send} = setup([{id: 't', kind: 'reasoning', text: 'a', at: 1000, seq: 1}]);
  act(() => root.querySelector('.reasoning-item summary').click());
  const box = root.querySelector('.reasoning-item-text');
  let height = 500;
  Object.defineProperty(box, 'scrollHeight', {get: () => height});
  Object.defineProperty(box, 'clientHeight', {get: () => 100});
  const scroll = top => {
    box.scrollTop = top;
    box.dispatchEvent(new Event('scroll'));
  };
  const stream = text => send([{id: 't', kind: 'reasoning', text, at: 1000, seq: 1}]);
  height = 600;
  stream('ab');
  expect(box.scrollTop).toBe(600);
  scroll(100);
  height = 700;
  stream('abc');
  expect(box.scrollTop).toBe(100);
  scroll(600);
  height = 800;
  stream('abcd');
  expect(box.scrollTop).toBe(800);
});

const png = 'iVBORw0KGgo=';

test('prompts, text, and tool calls show their images, outside the call\'s details', () => {
  const {root} = setup([
    {id: 'u', kind: 'user', text: 'look', images: [{mime_type: 'image/png', data: png}]},
    {id: 'x', kind: 'text', images: [{mime_type: 'image/gif', data: 'R0lGODlh'}]},
    {id: 'c', kind: 'tool', tool: 'screenshot', done: true, images: [{mime_type: 'image/png', data: png}, {mime_type: 'image/svg+xml', data: png}]},
  ]);
  const sources = selector => [...root.querySelectorAll(selector + ' img')].map(img => img.getAttribute('src'));
  expect(sources('.message-item')).toEqual(['data:image/png;base64,' + png]);
  expect(sources('.transcript-text')).toEqual(['data:image/gif;base64,R0lGODlh']);
  expect(sources('.tool-items-call')).toEqual(['data:image/png;base64,' + png]);
  expect(sources('.tool-items-call details')).toEqual([]);
});

test('mirrored requests name their sender and answer at its session ID; child status stays hidden', async () => {
  const fetches = spyOn(globalThis, 'fetch').mockImplementation(async () => new Response(''));
  spies.push(fetches);
  const {store, root, send} = setup([
    {id: 'status', kind: 'status', from: 'child', text: 'Reading.', at: 1},
    {id: 'request', kind: 'attention', from: 'child', text: 'Proceed?', choices: ['yes']},
    {id: 'message', kind: 'user', from: 'child', text: 'Result.'},
  ]);
  store.update(state => ({...state, workspaces: [{sessions: [{id: 's', children: [{id: 'child', parent: 's', title: 'Builder', children: []}]}]}]}));
  expect(root.textContent).not.toContain('Reading.');
  expect(store.get().session.transcript.items.status.text).toBe('Reading.');
  expect(root.querySelector('.attention-item-source').textContent).toBe('Builder');
  expect(root.querySelector('.disclosure-heading').textContent).toBe('From Builder');
  await act(async () => choose(root, 'yes'));
  expect(new URL(fetches.mock.calls[0][0], 'http://x/').searchParams.get('id')).toBe('child');
  send([{id: 'request', kind: 'attention', from: 'child', text: 'Proceed?', done: true, answer: 'yes'}]);
  expect(root.querySelector('.attention-item')).toBeNull();
  expect(root.textContent).toContain('Result.');
});

test('saved completed child requests are hidden while the session\'s own answers remain', () => {
  const {root} = setup([
    {id: 'child-request', kind: 'attention', from: 'child', text: 'Child approval?', done: true, answer: 'Allow'},
    {id: 'own-request', kind: 'attention', text: 'Own approval?', done: true, answer: 'Deny'},
  ]);
  expect(root.textContent).not.toContain('Child approval?');
  expect(root.textContent).not.toContain('Allow');
  expect(root.querySelector('.attention-item-answered').textContent).toBe('Own approval?');
  expect(root.querySelector('.attention-item-answer').textContent).toBe('Deny');
});
