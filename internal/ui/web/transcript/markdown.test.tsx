import {expect, test} from 'bun:test';
import {render} from '../../test-render.tsx';
import {Markdown, safeHref} from './markdown.tsx';

const markdown = (text: string): Element => {
  const root = document.createElement('div');
  render(<Markdown text={text} style="chat" />, root);
  const rendered = root.firstElementChild;
  if (!rendered) throw new Error('Markdown rendered nothing');
  return rendered;
};
const tags = (root: Element) => [...root.querySelectorAll('*')].map(e => e.tagName.toLowerCase());

test('blocks and inline marks', () => {
  const root = markdown('# Title\n\nFirst **bold** *it* ~~gone~~ `code`\n\n> quoted\n\n---\n\n```js\nlet a = 1;\n```');
  expect(tags(root).filter(tag => !['svg', 'rect', 'path', 'polyline'].includes(tag))).toEqual(['h1', 'p', 'strong', 'em', 'del', 'code', 'blockquote', 'p', 'hr', 'div', 'pre', 'code', 'button']);
  expect(root.querySelector('pre')?.textContent).toBe('let a = 1;');
});

test('a code block still being streamed runs to the end', () => {
  expect(markdown('Look:\n```\nx = 1\ny').querySelector('pre')?.textContent).toBe('x = 1\ny');
});

test('lists nest, keep their start, and show tasks', () => {
  const root = markdown('3. three\n   - inner\n4. four\n\n- [x] done\n- [ ] open');
  expect(root.querySelector('ol')?.getAttribute('start')).toBe('3');
  expect(root.querySelector('ol ul')?.children.length).toBe(1);
  expect([...root.querySelectorAll('input')].map(box => box.checked)).toEqual([true, false]);
});

test('tables keep their alignment', () => {
  const root = markdown('| Time | Event |\n|---|:-:|\n| 06:18 | **restart** |');
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

test('chat images keep their existing link or text presentation', () => {
  const root = markdown('![Remote](https://example.com/picture.png) ![Local](images/picture.png)');
  expect(root.querySelector('img')).toBeNull();
  expect(root.querySelector('a')?.getAttribute('href')).toBe('https://example.com/picture.png');
  expect(root.textContent).toBe('Remote Local');
});

test('Markdown from a model never becomes HTML', () => {
  const hostile = '<img src=x onerror=alert(1)> <script>alert(2)</script> **bold** [click](javascript:alert(3)) [ok](https://example.com/?q=<b>)\n\n```\n<iframe></iframe>\n```\n\n# <h1>';
  const root = markdown(hostile);
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
