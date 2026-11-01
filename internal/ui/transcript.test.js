import {expect, test} from 'bun:test';
import {Transcript, blocks, inline, renderMarkdown, safeHref, submitsOnKey, toolSummary} from './static/transcript.js';

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

test('blocks: paragraphs, headings, quotes, code', () => {
  const parsed = blocks('# Title\n\nFirst line\nsecond line\n\n> quoted\n> text\n\n```js\nlet a = 1;\n\nlet b = 2;\n```\n\nafter');
  expect(parsed.map(b => b.type)).toEqual(['heading', 'paragraph', 'quote', 'code', 'paragraph']);
  expect(parsed[0].level).toBe(1);
  expect(parsed[1].children.map(n => n.type)).toEqual(['text', 'break', 'text']);
  expect(parsed[2].children[0].type).toBe('paragraph');
  expect(parsed[3]).toEqual({type: 'code', lang: 'js', text: 'let a = 1;\n\nlet b = 2;'});
});

test('blocks: a code block still being streamed runs to the end', () => {
  expect(blocks('Look:\n```\nx = 1\ny')).toEqual([
    {type: 'paragraph', children: [{type: 'text', text: 'Look:'}]},
    {type: 'code', lang: '', text: 'x = 1\ny'},
  ]);
  // A shorter fence inside a longer one is code.
  expect(blocks('````\n```\n````')[0].text).toBe('```');
});

test('blocks: lists nest, continue past blank lines, and keep their start', () => {
  const [list] = blocks('- a\n  - inner\n  - inner two\n- b\n\n- c');
  expect(list.type).toBe('ul');
  expect(list.items.length).toBe(3);
  expect(list.items[0].map(b => b.type)).toEqual(['paragraph', 'ul']);
  expect(list.items[0][1].items.length).toBe(2);

  const [ordered, after] = blocks('3. three\n4. four\ntext');
  expect(ordered).toMatchObject({type: 'ol', start: 3});
  // A line with no marker continues the last item.
  expect(after).toBeUndefined();
  expect(ordered.items[1][0].children.map(n => n.text ?? n.type)).toEqual(['four', 'break', 'text']);

  expect(blocks('- a\n1. b').map(b => b.type)).toEqual(['ul', 'ol']);
});

test('inline: code, emphasis, escapes, and plain asterisks and underscores', () => {
  expect(inline('use `a*b` and **bold** and *it* and _it_')).toEqual([
    {type: 'text', text: 'use '}, {type: 'code', text: 'a*b'},
    {type: 'text', text: ' and '}, {type: 'strong', children: [{type: 'text', text: 'bold'}]},
    {type: 'text', text: ' and '}, {type: 'em', children: [{type: 'text', text: 'it'}]},
    {type: 'text', text: ' and '}, {type: 'em', children: [{type: 'text', text: 'it'}]},
  ]);
  expect(inline('2 * 3 * 4 and snake_case_name')).toEqual([{type: 'text', text: '2 * 3 * 4 and snake_case_name'}]);
  expect(inline('\\*not\\* emphasis')).toEqual([{type: 'text', text: '*not* emphasis'}]);
  expect(inline('``a ` b``')).toEqual([{type: 'code', text: 'a ` b'}]);
  expect(inline('unclosed `code and **bold')).toEqual([{type: 'text', text: 'unclosed `code and **bold'}]);
});

test('inline: links and bare addresses', () => {
  expect(inline('[docs](https://example.com/a) and https://example.org/b.')).toEqual([
    {type: 'link', href: 'https://example.com/a', children: [{type: 'text', text: 'docs'}]},
    {type: 'text', text: ' and '},
    {type: 'link', href: 'https://example.org/b', children: [{type: 'text', text: 'https://example.org/b'}]},
    {type: 'text', text: '.'},
  ]);
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
  const root = new El('div');
  root.append(...renderMarkdown(doc, hostile));
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
  const allowed = new Set(['div', 'details', 'summary', 'span', 'pre']);
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
