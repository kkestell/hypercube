import {afterEach, expect, spyOn, test} from 'bun:test';
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
});

function setup(items) {
  const store = createStore({workspaces: [], drafts: {}, errors: [], session: emptySession('s')});
  const send = (items, reset = false) => store.update(s => receivedTranscript(s, {session: 's', agent: 'a', reset, items}));
  send(withSeq(items), true);
  const root = mount(store, <Transcript session={{id: "s"}} />);
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

test('an attention request offers its choices, waits while sending, and opens again when refused', async () => {
  let answer;
  const fetches = spyOn(globalThis, 'fetch').mockImplementation(() => new Promise(resolve => { answer = resolve; }));
  spies.push(fetches);
  const {root, send} = setup([{id: 'q', kind: 'attention', text: 'Proceed?', choices: ['yes', 'no']}]);
  const buttons = () => [...root.querySelectorAll('vscode-button')];
  expect(buttons().map(b => b.textContent)).toEqual(['yes', 'no']);
  act(() => buttons()[0].click());
  const url = new URL(fetches.mock.calls[0][0], 'http://x/');
  expect(Object.fromEntries(url.searchParams)).toEqual({action: 'answer', id: 's', request: 'q', choice: 'yes'});
  expect(buttons().every(b => b.hasAttribute('disabled'))).toBe(true);
  await act(async () => {
    answer(new Response('no longer open', {status: 409}));
    await new Promise(resolve => setTimeout(resolve));
  });
  expect(buttons().every(b => !b.hasAttribute('disabled'))).toBe(true);

  send([{id: 'q', kind: 'attention', text: 'Proceed?', done: true, answer: 'no', seq: 1}]);
  expect(buttons()).toEqual([]);
  expect(root.querySelector('.attention-item-answered').textContent).toBe('Proceed?');
  expect(root.querySelector('.attention-item-answer').textContent).toBe('no');
});

test('a typed answer is sent with Enter and not when empty', async () => {
  const fetches = spyOn(globalThis, 'fetch').mockImplementation(async () => new Response(''));
  spies.push(fetches);
  const {root} = setup([{id: 'q', kind: 'attention', text: 'Name?', free: true}]);
  const field = root.querySelector('vscode-textfield');
  act(() => field.dispatchEvent(new KeyboardEvent('keydown', {key: 'Enter'})));
  expect(fetches).not.toHaveBeenCalled();
  field.value = 'Ada';
  act(() => field.dispatchEvent(new Event('input')));
  await act(async () => field.dispatchEvent(new KeyboardEvent('keydown', {key: 'Enter'})));
  const [url, options] = fetches.mock.calls[0];
  expect(new URL(url, 'http://x/').searchParams.get('choice')).toBe('');
  expect(options.body).toBe('Ada');
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

test('long messages between sessions expand and collapse; the user\'s own prompts do not clamp', () => {
  const {root} = setup([
    {id: 'in', kind: 'user', from: 'c1', text: longText, done: true},
    {id: 'out', kind: 'tool', server: 'hypercube', tool: 'send_message', input: {to: 'c1', text: longText}},
    {id: 'own', kind: 'user', text: longText},
  ]);
  const [incoming, outgoing, own] = root.querySelectorAll('.message-item');
  expect(incoming.textContent).toStartWith('From c1');
  expect(outgoing.textContent).toStartWith('To c1');
  expect(outgoing.querySelector('[title="Running"]')).not.toBeNull();
  for (const message of [incoming, outgoing]) {
    const text = message.querySelector('.message-item-text');
    expect(text.classList.contains('message-item-clamped')).toBe(true);
    act(() => message.querySelector('.message-item-more').click());
    expect(text.classList.contains('message-item-clamped')).toBe(false);
    expect(message.querySelector('.message-item-more').textContent).toBe('Show less');
  }
  expect(own.querySelector('.message-item-clamped')).toBeNull();
  expect(own.querySelector('.message-item-more')).toBeNull();
});

test('Thinking shows a preview until expanded and keeps the choice while it streams', () => {
  const {root, send} = setup([{id: 't', kind: 'reasoning', text: longText}]);
  const preview = () => root.querySelector('.reasoning-item-text').classList.contains('reasoning-item-preview');
  expect(preview()).toBe(true);
  act(() => root.querySelector('.reasoning-item-toggle').click());
  expect(preview()).toBe(false);
  send([{id: 't', kind: 'reasoning', text: longText + '\nmore', seq: 1}]);
  expect(preview()).toBe(false);
  expect(root.querySelector('.reasoning-item-text').textContent).toEndWith('more');
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

test('mirrored requests name their sender and answer at its session ID; status rows name the sender', async () => {
  const fetches = spyOn(globalThis, 'fetch').mockImplementation(async () => new Response(''));
  spies.push(fetches);
  const {store, root} = setup([
    {id: 'status', kind: 'status', from: 'child', text: 'Reading.', at: 1},
    {id: 'request', kind: 'attention', from: 'child', text: 'Proceed?', choices: ['yes']},
    {id: 'message', kind: 'user', from: 'child', text: 'Result.'},
  ]);
  store.update(state => ({...state, workspaces: [{sessions: [{id: 's', children: [{id: 'child', parent: 's', title: 'Builder', children: []}]}]}]}));
  expect(root.querySelector('.status-item-source').textContent).toBe('Builder');
  expect(root.querySelector('.status-item [name="pulse"]')).not.toBeNull();
  expect(root.querySelector('.attention-item-source').textContent).toBe('Builder');
  expect(root.querySelector('.message-item-heading').textContent).toBe('From Builder');
  await act(async () => root.querySelector('.attention-form vscode-button').click());
  expect(new URL(fetches.mock.calls[0][0], 'http://x/').searchParams.get('id')).toBe('child');
});
