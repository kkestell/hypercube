import {expect, test} from 'bun:test';
import {Transcript, isLong, renderMarkdown, safeHref, submitsOnKey, toolSummary, toolTitle} from './transcript.js';

// A minimal document: the renderer builds nodes and never parses HTML.
class Text {
  constructor(text) { this.text = text; }
  get textContent() { return this.text; }
}

class El {
  constructor(tag) {
    this.tag = tag;
    this.children = [];
    this.attributes = {};
    this.className = '';
    this.hidden = false;
    this.parentNode = null;
    this.value = '';
    this.scrollTop = 0;
    this.scrollHeight = 0;
    this.clientHeight = 0;
  }
  append(...nodes) {
    for (const n of nodes) {
      const node = typeof n === 'string' ? new Text(n) : n;
      if (node instanceof El) node.parentNode = this;
      this.children.push(node);
    }
  }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  replaceWith(node) {
    this.parentNode.children[this.parentNode.children.indexOf(this)] = node;
    node.parentNode = this.parentNode;
  }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  get textContent() { return this.children.map(c => c.textContent).join(''); }
  set textContent(value) { this.children = value === '' ? [] : [new Text(value)]; }
}

const doc = {createElement: tag => new El(tag), createTextNode: text => new Text(text)};

const elements = node => node instanceof El ? [node, ...node.children.flatMap(elements)] : [];
const tags = node => elements(node).map(e => e.tag);
const find = (node, tag) => elements(node).filter(e => e.tag === tag);

const markdown = text => {
  const root = new El('div');
  root.append(...renderMarkdown(doc, text));
  return root;
};

test('markdown: blocks and inline marks', () => {
  const root = markdown('# Title\n\nFirst **bold** *it* ~~gone~~ `code`\n\n> quoted\n\n---\n\n```js\nlet a = 1;\n```');
  expect(tags(root)).toEqual(['div', 'h1', 'p', 'strong', 'em', 'del', 'code', 'blockquote', 'p', 'hr', 'pre', 'code']);
  expect(find(root, 'pre')[0].textContent).toBe('let a = 1;');
});

test('markdown: a code block still being streamed runs to the end', () => {
  expect(find(markdown('Look:\n```\nx = 1\ny'), 'pre')[0].textContent).toBe('x = 1\ny');
});

test('markdown: lists nest, keep their start, and show tasks', () => {
  const root = markdown('3. three\n   - inner\n4. four\n\n- [x] done\n- [ ] open');
  const [ordered, inner, tasks] = find(root, 'ol').concat(find(root, 'ul'));
  expect(ordered.attributes.start).toBe('3');
  expect(inner.children.length).toBe(1);
  expect(find(tasks, 'input').map(b => b.checked)).toEqual([true, false]);
});

test('markdown: tables keep their alignment', () => {
  const root = markdown('| Time | Event |\n|---|:-:|\n| 06:18 | **restart** |');
  expect(find(root, 'th').map(c => c.textContent)).toEqual(['Time', 'Event']);
  const cells = find(root, 'td');
  expect(cells.map(c => c.textContent)).toEqual(['06:18', 'restart']);
  expect(cells[1].attributes.style).toBe('text-align: center');
});

test('safeHref allows only web and mail addresses', () => {
  expect(safeHref('https://example.com/a b')).toBe('https://example.com/a%20b');
  expect(safeHref('http://example.com')).toBe('http://example.com/');
  expect(safeHref('mailto:a@example.com')).toBe('mailto:a@example.com');
  for (const bad of ['javascript:alert(1)', ' JaVaScRiPt:alert(1)', '\tjavascript:alert(1)', 'data:text/html,<b>', 'file:///etc/passwd', '/relative', '//example.com', '']) {
    expect(safeHref(bad)).toBeNull();
  }
});

test('markdown from a model never becomes HTML', () => {
  const hostile = '<img src=x onerror=alert(1)> <script>alert(2)</script> **bold** [click](javascript:alert(3)) [ok](https://example.com/?q=<b>)\n\n```\n<iframe></iframe>\n```\n\n# <h1>';
  const root = markdown(hostile);
  const allowed = new Set(['div', 'p', 'strong', 'em', 'span', 'a', 'br', 'code', 'pre', 'h1', 'blockquote', 'ul', 'ol', 'li']);
  expect(tags(root).filter(t => !allowed.has(t))).toEqual([]);
  expect(root.textContent).toContain('<img src=x onerror=alert(1)> <script>alert(2)</script>');
  expect(root.textContent).toContain('<iframe></iframe>');
  const links = find(root, 'a');
  expect(links.length).toBe(1);
  expect(links[0].attributes.href).toBe('https://example.com/?q=%3Cb%3E');
  expect(links[0].attributes.rel).toBe('noopener noreferrer');
  // The unsafe link keeps its text and loses its address.
  expect(root.textContent).toContain('click');
  expect(JSON.stringify(elements(root).map(e => e.attributes))).not.toContain('javascript');
});

test('toolSummary picks the telling text of a call', () => {
  expect(toolSummary({command: 'ls -la\n  /tmp', timeout: 5})).toBe('ls -la /tmp');
  expect(toolSummary({file_path: '/a/b.go', content: 'x'})).toBe('/a/b.go');
  expect(toolSummary({n: 1, label: 'first string'})).toBe('first string');
  expect(toolSummary('plain')).toBe('plain');
  expect(toolSummary({n: 1})).toBe('');
  expect(toolSummary(undefined)).toBe('');
  expect(toolSummary({command: 'x'.repeat(200)}).length).toBe(120);
});

test('submitsOnKey sends on Enter alone', () => {
  expect(submitsOnKey({key: 'Enter'})).toBe(true);
  expect(submitsOnKey({key: 'Enter', shiftKey: true})).toBe(false);
  expect(submitsOnKey({key: 'Enter', isComposing: true})).toBe(false);
  expect(submitsOnKey({key: 'Enter', metaKey: true})).toBe(false);
  expect(submitsOnKey({key: 'a'})).toBe(false);
});

const answers = [];
const transcript = (accept = true) => new Transcript(doc, {answer: async (item, choice, text) => { answers.push({id: item.id, choice, text}); return accept; }});
const shown = t => t.list.children.map(c => c.tag + ':' + c.textContent);

test('items replace themselves in place, and unchanged items are left alone', () => {
  const t = transcript();
  t.reset([{id: '1', kind: 'user', text: 'hi'}, {id: '2', kind: 'text', text: 'Hel'}]);
  expect(t.list.children.length).toBe(2);
  const [user, reply] = t.list.children;
  expect(user.textContent).toBe('hi');
  expect(reply.textContent).toBe('Hel');

  t.update([{id: '2', kind: 'text', text: 'Hello **there**'}, {id: '3', kind: 'text', text: 'next'}]);
  expect(t.list.children.length).toBe(3);
  expect(t.list.children[0]).toBe(user);
  expect(t.list.children[1]).toBe(reply);
  expect(reply.textContent).toBe('Hello there');
  expect(find(reply, 'strong').length).toBe(1);

  const replyBody = reply.children[0];
  t.update([{id: '2', kind: 'text', text: 'Hello **there**'}]);
  expect(reply.children[0]).toBe(replyBody);

  t.reset([{id: '9', kind: 'error', text: 'boom'}]);
  expect(shown(t)).toEqual(['div:boom']);
});

test('a tool call shows its summary, status, and nested items', () => {
  const t = transcript();
  t.reset([{id: 'c', kind: 'tool', tool: 'bash', input: {command: 'ls'}}]);
  const [call] = t.list.children;
  expect(call.tag).toBe('details');
  const [summary, input, output, kids] = call.children;
  expect(summary.textContent).toContain('bash');
  expect(summary.textContent).toContain('ls');
  expect(find(summary, 'span').some(s => s.className.includes('icon-loading'))).toBe(true);
  expect(input.textContent).toBe('{\n  "command": "ls"\n}');
  expect(output.hidden).toBe(true);
  expect(kids.hidden).toBe(true);

  t.update([
    {id: 'n', parent: 'c', kind: 'text', text: 'inside'},
    {id: 'c', kind: 'tool', tool: 'bash', input: {command: 'ls'}, output: 'a.txt', done: true},
  ]);
  expect(t.list.children.length).toBe(1);
  expect(kids.hidden).toBe(false);
  expect(kids.textContent).toBe('inside');
  expect(output.textContent).toBe('a.txt');
  expect(output.hidden).toBe(false);
  expect(find(summary, 'span').some(s => s.className.includes('icon-loading'))).toBe(false);

  t.update([{id: 'c', kind: 'tool', tool: 'bash', input: {command: 'ls'}, output: 'oops', done: true, failed: true}]);
  expect(find(summary, 'span').some(s => s.title === 'Failed')).toBe(true);
  expect(kids.textContent).toBe('inside');
});

test('tool and error text is shown as text', () => {
  const t = transcript();
  t.reset([
    {id: 'c', kind: 'tool', tool: '<b>tool</b>', input: {command: '<script>x</script>'}, output: '<img src=x onerror=1>', done: true},
    {id: 'e', kind: 'error', text: '<u>bad</u>'},
    {id: 'r', kind: 'reasoning', text: '<i>thinking</i>'},
    {id: 'u', kind: 'user', text: '<a href=x>me</a>'},
  ]);
  const allowed = new Set(['div', 'details', 'summary', 'span', 'pre', 'button']);
  expect(tags(t.root).filter(tag => !allowed.has(tag))).toEqual([]);
  expect(t.root.textContent).toContain('<b>tool</b>');
  expect(t.root.textContent).toContain('<img src=x onerror=1>');
  expect(t.root.textContent).toContain('<u>bad</u>');
  expect(t.root.textContent).toContain('<i>thinking</i>');
  expect(t.root.textContent).toContain('<a href=x>me</a>');
});

test('an attention request offers its choices until it finishes', async () => {
  answers.length = 0;
  const t = transcript();
  t.reset([{id: 'q', kind: 'attention', text: 'Proceed?', choices: ['yes', 'no']}]);
  const buttons = () => find(t.root, 'button');
  expect(buttons().map(b => b.textContent)).toEqual(['yes', 'no']);
  expect(t.root.textContent).toContain('Proceed?');

  const [yes, no] = buttons();
  yes.onclick();
  expect(answers).toEqual([{id: 'q', choice: 'yes', text: ''}]);
  expect(buttons().every(b => b.disabled)).toBe(true);
  await Promise.resolve();
  await Promise.resolve();
  expect(buttons().every(b => b.disabled)).toBe(true);

  // An update that leaves the request open keeps its controls.
  t.update([{id: 'q', kind: 'attention', text: 'Proceed?', choices: ['yes', 'no']}]);
  expect(buttons()[1]).toBe(no);

  t.update([{id: 'q', kind: 'attention', text: 'Proceed?', done: true, answer: 'yes'}]);
  expect(buttons()).toEqual([]);
  expect(t.root.textContent).toContain('yes');
});

test('a refused answer re-enables the controls', async () => {
  answers.length = 0;
  const t = transcript(false);
  t.reset([{id: 'q', kind: 'attention', text: 'Name?', choices: ['skip'], free: true}]);
  const [input] = find(t.root, 'input');
  const [skip, send] = find(t.root, 'button');
  // An empty reply is not sent.
  send.onclick();
  expect(answers).toEqual([]);
  input.value = 'Ada';
  input.onkeydown({key: 'Enter'});
  expect(answers).toEqual([{id: 'q', choice: '', text: 'Ada'}]);
  expect(input.disabled).toBe(true);
  await Promise.resolve();
  await Promise.resolve();
  expect(input.disabled).toBe(false);
  expect(skip.disabled).toBe(false);
  skip.onclick();
  expect(answers[1]).toEqual({id: 'q', choice: 'skip', text: 'Ada'});
});

test('a request inside a collapsed tool opens it', () => {
  const t = transcript();
  t.reset([
    {id: 'a', kind: 'tool', tool: 'agent'},
    {id: 'b', parent: 'a', kind: 'tool', tool: 'inner'},
    {id: 'q', parent: 'b', kind: 'attention', text: 'Allow?', choices: ['ok']},
  ]);
  const [outer] = t.list.children;
  const inner = find(outer, 'details')[1];
  expect(outer.open).toBe(true);
  expect(inner.open).toBe(true);
});

test('the view follows new items only from the end', () => {
  const t = transcript();
  t.root.scrollHeight = 1000;
  t.root.clientHeight = 300;
  t.reset([{id: '1', kind: 'text', text: 'a'}]);
  expect(t.root.scrollTop).toBe(1000);

  t.root.scrollHeight = 1200;
  t.update([{id: '2', kind: 'text', text: 'b'}]);
  expect(t.root.scrollTop).toBe(1200);

  t.root.scrollTop = 100;
  t.root.scrollHeight = 1500;
  t.update([{id: '3', kind: 'text', text: 'c'}]);
  expect(t.root.scrollTop).toBe(100);
});

test('toolTitle says what Hypercube tools do and names other servers', () => {
  expect(toolTitle({server: 'hypercube', tool: 'remove_agent', input: {name: 'auditor'}})).toEqual(['Remove agent', 'auditor']);
  expect(toolTitle({server: 'hypercube', tool: 'create_agent', input: {name: 'w1', profile: 'claude', message: 'the brief'}})).toEqual(['Create agent', 'w1']);
  expect(toolTitle({server: 'hypercube', tool: 'list_agents', input: {}})).toEqual(['List agents', '']);
  expect(toolTitle({server: 'hypercube', tool: 'set_status', input: {text: 'Reading\n the code'}})).toEqual(['Set status', 'Reading the code']);
  expect(toolTitle({server: 'hypercube', tool: 'remove_agent'})).toEqual(['Remove agent', '']);
  expect(toolTitle({server: 'context7', tool: 'query-docs', input: {query: 'go'}})).toEqual(['context7 - query-docs', 'go']);
  expect(toolTitle({tool: 'bash', input: {command: 'ls'}})).toEqual(['bash', 'ls']);
});

const longText = Array.from({length: 10}, (_, i) => 'line ' + i).join('\n');
const clamped = body => body.className.includes('line-clamp');

test('a message an agent sends shows its recipient and text', () => {
  const t = transcript();
  t.reset([{id: 's', kind: 'tool', server: 'hypercube', tool: 'send_message', input: {to: 'c1/lead', text: 'Done.'}}]);
  const [row] = t.list.children;
  expect(row.textContent).toBe('To c1/leadDone.');
  expect(find(row, 'span').some(s => s.className.includes('icon-loading'))).toBe(true);
  expect(find(row, 'button')[0].hidden).toBe(true);

  t.update([{id: 's', kind: 'tool', server: 'hypercube', tool: 'send_message', input: {to: 'c1/lead', text: 'Done.'}, output: 'no such agent', done: true, failed: true}]);
  expect(t.list.children[0]).toBe(row);
  expect(find(row, 'span').some(s => s.className.includes('icon-loading'))).toBe(false);
  expect(row.textContent).toContain('no such agent');
});

test('a long message between agents expands and collapses', () => {
  const t = transcript();
  t.reset([
    {id: 'in', kind: 'user', from: 'c1/lead', text: longText, done: true},
    {id: 'out', kind: 'tool', server: 'hypercube', tool: 'send_message', input: {to: 'c1/lead', text: longText}},
  ]);
  for (const row of t.list.children) {
    const [bubble] = row.children;
    const [, body, more] = bubble.children;
    expect(body.textContent).toBe(longText);
    expect(clamped(body)).toBe(true);
    expect(more.hidden).toBe(false);
    expect(more.textContent).toBe('Show more');
    more.onclick();
    expect(clamped(body)).toBe(false);
    expect(more.textContent).toBe('Show less');
    more.onclick();
    expect(clamped(body)).toBe(true);
  }
  expect(t.list.children[0].textContent).toStartWith('From c1/lead');

  // An expanded message stays expanded when its call finishes.
  const out = t.list.children[1];
  const [, body, more] = out.children[0].children;
  more.onclick();
  t.update([{id: 'out', kind: 'tool', server: 'hypercube', tool: 'send_message', input: {to: 'c1/lead', text: longText}, done: true}]);
  expect(clamped(body)).toBe(false);
});

test('the user\'s own prompts are shown whole', () => {
  const t = transcript();
  t.reset([{id: 'u', kind: 'user', text: longText}]);
  const [, body, more] = t.list.children[0].children[0].children;
  expect(clamped(body)).toBe(false);
  expect(more.hidden).toBe(true);
  expect(t.list.children[0].textContent).toBe(longText);
});

test('isLong counts lines and length', () => {
  expect(isLong('a\nb')).toBe(false);
  expect(isLong(longText)).toBe(true);
  expect(isLong('x'.repeat(601))).toBe(true);
});
