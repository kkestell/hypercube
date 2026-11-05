import {afterEach, expect, test} from 'bun:test';
import {act} from 'react';
import {render, unmountAll} from '../../test-render.tsx';
import {Markdown, MarkdownText, safeHref} from './markdown.tsx';

afterEach(unmountAll);

const markdown = async (text: string): Promise<Element> => {
  const root = document.createElement('div');
  await act(async () => { render(<Markdown text={text} style="chat" />, root); });
  const rendered = root.firstElementChild;
  if (!rendered) throw new Error('Markdown rendered nothing');
  return rendered;
};
const tags = (root: Element) => [...root.querySelectorAll('*')].map(e => e.tagName.toLowerCase());

test('blocks and inline marks', async () => {
  const root = await markdown('# Title\n\nFirst **bold** *it* ~~gone~~ `code`\n\n> quoted\n\n---\n\n```js\nlet a = 1;\n```');
  expect(root.querySelector('h1')?.textContent).toBe('Title');
  expect(root.querySelector('strong')?.textContent).toBe('bold');
  expect(root.querySelector('em')?.textContent).toBe('it');
  expect(root.querySelector('del')?.textContent).toBe('gone');
  expect(root.querySelector('blockquote')?.textContent).toContain('quoted');
  expect(root.querySelector('pre')?.textContent).toBe('let a = 1;');
});

test('a code block still being streamed runs to the end', async () => {
  expect((await markdown('Look:\n```\nx = 1\ny')).querySelector('pre')?.textContent).toBe('x = 1\ny');
});

test('lists nest, keep their start, and show tasks', async () => {
  const root = await markdown('3. three\n   - inner\n4. four\n\n- [x] done\n- [ ] open');
  expect(root.querySelector('ol')?.getAttribute('start')).toBe('3');
  expect(root.querySelector('ol ul')?.children.length).toBe(1);
  expect([...root.querySelectorAll('input')].map(box => box.checked)).toEqual([true, false]);
});

test('tables keep their alignment', async () => {
  const root = await markdown('| Time | Event |\n|---|:-:|\n| 06:18 | **restart** |');
  expect([...root.querySelectorAll('th')].map(c => c.textContent)).toEqual(['Time', 'Event']);
  const cells = [...root.querySelectorAll('td')];
  expect(cells.map(c => c.textContent)).toEqual(['06:18', 'restart']);
  expect(cells[1]?.style.textAlign).toBe('center');
});

test('safeHref allows only web and mail addresses', () => {
  expect(safeHref('https://example.com/a b')).toBe('https://example.com/a%20b');
  expect(safeHref('http://example.com')).toBe('http://example.com/');
  expect(safeHref('mailto:a@example.com')).toBe('mailto:a@example.com');
  for (const bad of ['javascript:alert(1)', ' JaVaScRiPt:alert(1)', '\tjavascript:alert(1)', 'data:text/html,<b>', 'file:///etc/passwd', '/relative', '//example.com', '']) {
    expect(safeHref(bad)).toBeNull();
  }
});

test('Markdown text decodes named and numeric entities once and keeps escaped ampersands', async () => {
  const text = String.raw`Tom &amp; Jerry &#35; &#x1F600; &amp;lt; \&amp; &copy &unknown;`;
  const expected = 'Tom & Jerry # 😀 &lt; &amp; &copy &unknown;';
  expect((await markdown(text)).textContent).toBe(expected);
  const root = document.createElement('div');
  render(<MarkdownText text={text} />, root);
  expect(root.textContent).toBe(expected);
});

test('entity decoding leaves code and raw HTML literal', async () => {
  const root = await markdown('`&amp; &#65;`\n\n```\n&amp; &#65;\n```\n\n<div title="&amp;">\n&copy;\n</div>');
  expect([...root.querySelectorAll('code')].map(code => code.textContent)).toEqual(['&amp; &#65;', '&amp; &#65;']);
  expect(root.textContent).toContain('<div title="&amp;">\n&copy;\n</div>');
  expect(root.querySelector('div[title]')).toBeNull();
});

test('link destinations decode entities before URL validation', async () => {
  const root = await markdown('[safe](https://example.com/?a=1&amp;b=2) [blocked](jav&#x61;script:alert%281%29)');
  const links = [...root.querySelectorAll('a')];
  expect(links.map(link => link.getAttribute('href'))).toEqual(['https://example.com/?a=1&b=2']);
  expect(root.textContent).toBe('safe blocked');
});

test('preview images decode their path, alternative text, and title', async () => {
  const root = document.createElement('div');
  const paths: string[] = [];
  render(<Markdown text={'![A &amp; B](images/a&amp;b.png "T &quot;Q&quot;")'} style="preview" imageSource={href => { paths.push(href); return '/image'; }} />, root);
  expect(paths).toEqual(['images/a&b.png']);
  const image = root.querySelector('img');
  expect(image?.alt).toBe('A & B');
  expect(image?.title).toBe('T "Q"');
});

test('chat images keep their existing link or text presentation', async () => {
  const root = await markdown('![Remote](https://example.com/picture.png) ![Local](images/picture.png)');
  expect(root.querySelector('img')).toBeNull();
  expect(root.querySelector('a')?.getAttribute('href')).toBe('https://example.com/picture.png');
  expect(root.textContent).toBe('Remote Local');
});

test('Markdown from a model never becomes HTML', async () => {
  const hostile = '<img src=x onerror=alert(1)> <script>alert(2)</script> **bold** [click](javascript:alert(3)) [ok](https://example.com/?q=<b>)\n\n```\n<iframe></iframe>\n```\n\n# <h1>';
  const root = await markdown(hostile);
  const allowed = new Set(['p', 'strong', 'em', 'span', 'a', 'br', 'code', 'pre', 'h1', 'div', 'button', 'svg', 'rect', 'path', 'polyline']);
  expect(tags(root).filter(t => !allowed.has(t))).toEqual([]);
  expect(root.textContent).toContain('<img src=x onerror=alert(1)> <script>alert(2)</script>');
  expect(root.textContent).toContain('<iframe></iframe>');
  const links = root.querySelectorAll('a');
  expect(links.length).toBe(1);
  expect(links[0]?.getAttribute('href')).toBe('https://example.com/?q=%3Cb%3E');
  expect(links[0]?.getAttribute('rel')).toBe('noopener noreferrer');
  expect(root.textContent).toContain('click');
  expect(root.innerHTML).not.toContain('javascript');
});
