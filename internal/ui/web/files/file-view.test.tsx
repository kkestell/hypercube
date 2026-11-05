import {afterEach, expect, mock, spyOn, test} from 'bun:test';
import {act} from 'react';
import {mount, render, unmountAll} from '../../test-render.tsx';
import {testState, unread} from '../../test-state.ts';
import {openTab} from '../editor/tab-layout.ts';
import {emptySession} from '../sessions/selection.ts';
import {createStore, StoreContext} from '../store.ts';
import {FileView} from './file-view.tsx';
import {setSource} from './files.ts';

afterEach(() => { unmountAll(); mock.restore(); });

test('Markdown file previews render images relative to the file and session', () => {
  const path = 'docs #1/guide.md';
  const session = emptySession('s & 1');
  session.files.open[path] = {...unread, text: `
- ![Local](images/picture.png "Caption")

| Image |
|---|
| ![Parent](../assets/picture%20%23.png) |

![Root](/assets/root.png)
![SVG](drawing.svg#part)
![Remote](https://example.com/picture.png?size=2)
![Blocked](javascript:alert%281%29)
![Encoded](jav&#x61;script:alert%281%29)
![File](file:///etc/passwd)
![HTML](data:text/html,hello)
<img src=x onerror=alert(1)>
`};
  const store = createStore(testState({session}));
  const root = document.createElement('div');
  try {
    render(<StoreContext.Provider value={[store.get(), store]}><FileView id="file" path={path} /></StoreContext.Provider>, root);
    const images = [...root.querySelectorAll('img')];
    expect(images.map(image => image.alt)).toEqual(['Local', 'Parent', 'Root', 'SVG', 'Remote']);
    const sources = images.map(image => new URL(image.getAttribute('src') ?? '', 'http://localhost:4280'));
    expect(sources.slice(0, 4).map(url => url.pathname)).toEqual(['/image', '/image', '/image', '/image']);
    expect(sources.slice(0, 4).map(url => url.searchParams.get('session'))).toEqual(Array(4).fill('s & 1'));
    expect(sources.slice(0, 4).map(url => url.searchParams.get('path'))).toEqual([
      'docs #1/images/picture.png', 'assets/picture #.png', 'assets/root.png', 'docs #1/drawing.svg',
    ]);
    expect(images[0].title).toBe('Caption');
    expect(sources[3].hash).toBe('#part');
    expect(sources[4].href).toBe('https://example.com/picture.png?size=2');
    expect(root.textContent).toContain('<img src=x onerror=alert(1)>');
  } finally {
    render(null, root);
  }
});

test.each([
  ['picture.png', 'image/png', 'img', 'src'],
  ['clip.mp4', 'video/mp4', 'video', 'src'],
  ['sound.mp3', 'audio/mpeg', 'audio', 'src'],
  ['document.pdf', 'application/pdf', 'object', 'data'],
  ['index.html', 'text/html', 'iframe', 'src'],
])('browser preview for %s uses its native viewer', (path, mime, tag, attribute) => {
  const session = emptySession('s');
  const url = '/preview/s/repo/' + path + '?v=1';
  session.files.open[path] = {...unread, preview: {mime, url}};
  const root = mount(createStore(testState({session})), <FileView id={'file:' + path} path={path} />);
  const viewer = root.querySelector(tag);
  expect(viewer?.getAttribute(attribute)).toBe(url);
  if (tag === 'iframe') expect(viewer?.getAttribute('sandbox')).toBe('');
  if (tag === 'audio' || tag === 'video') expect(viewer?.hasAttribute('controls')).toBe(true);
  expect(root.querySelector('.file-view-code')).toBeNull();
});

test('textual browser previews switch to highlighted source, then refresh the native viewer', () => {
  const path = 'drawing.svg';
  const session = emptySession('s');
  session.files.open[path] = {...unread, text: '<svg/>', preview: {mime: 'image/svg+xml', url: '/preview/s/repo/drawing.svg?v=1'}};
  const store = createStore(testState({session}));
  const root = mount(store, <FileView id={'file:' + path} path={path} />);
  expect(root.querySelector('img')).not.toBeNull();
  act(() => store.update(state => setSource(state, path, true)));
  expect(root.querySelector('img')).toBeNull();
  expect(root.querySelector('code')?.textContent).toBe('<svg/>');
  act(() => store.update(state => setSource(state, path, false)));
  act(() => store.update(state => ({...state, session: {...session, files: {...session.files, open: {
    [path]: {...session.files.open[path], preview: {mime: 'image/svg+xml', url: '/preview/s/repo/drawing.svg?v=2'}},
  }}}})));
  expect(root.querySelector('img')?.getAttribute('src')).toBe('/preview/s/repo/drawing.svg?v=2');
});

test('source scrolling after a preview switch is remembered across remounts', () => {
  const path = 'drawing.svg', id = 'file:' + path;
  const session = emptySession('s');
  session.tabs = openTab(session.tabs, id);
  session.files.open[path] = {...unread, text: '<svg/>', preview: {mime: 'image/svg+xml', url: '/preview/s/repo/drawing.svg?v=1'}};
  const store = createStore(testState({session}));
  const root = mount(store, <FileView id={id} path={path} />);
  act(() => store.update(state => setSource(state, path, true)));
  const scroller = root.querySelector<HTMLElement>('.file-view')!;
  act(() => {
    scroller.scrollTop = 500;
    scroller.dispatchEvent(new Event('scroll'));
  });
  render(null, root);
  expect(store.get().session?.scroll[id]?.top).toBe(500);
  const restored = mount(store, <FileView id={id} path={path} />);
  expect(restored.querySelector('.file-view')?.scrollTop).toBe(500);
});

test('a browser decoding error offers the file directly', () => {
  const path = 'unsupported.png';
  const session = emptySession('s');
  session.files.open[path] = {...unread, preview: {mime: 'image/png', url: '/preview/s/repo/unsupported.png?v=1'}};
  const root = mount(createStore(testState({session})), <FileView id={'file:' + path} path={path} />);
  act(() => root.querySelector('img')?.dispatchEvent(new Event('error')));
  expect(root.textContent).toContain('This browser could not display the file.');
  expect(root.querySelector('a')?.getAttribute('href')).toBe('/preview/s/repo/unsupported.png?v=1');
});

test('source highlights reveal once after text arrives, across refreshes and remounts', () => {
  const scroll = spyOn(Element.prototype, 'scrollIntoView').mockImplementation(() => {});
  try {
    const session = emptySession('s');
    session.files.open['a.md'] = {...unread, source: true, lines: [{start: 1, end: 2}, {start: 3, end: 3}], reveal: true};
    session.scroll['file:a.md'] = {top: 100, left: 0, following: false};
    const store = createStore(testState({session}));
    const root = mount(store, <FileView id="file:a.md" path="a.md" />);
    expect(root.querySelectorAll('.file-view-highlight')).toHaveLength(0);
    expect(scroll).not.toHaveBeenCalled();
    const text = (value: string) => store.update(state => state.session ? {...state, session: {...state.session, files: {...state.session.files, open: {
      'a.md': {...unread, ...state.session.files.open['a.md'], text: value},
    }}}} : state);
    act(() => text('# Title\nline\nlast'));
    expect(root.querySelector('.markdown-preview')).toBeNull();
    const highlights = root.querySelectorAll('.file-view-highlight');
    expect(highlights).toHaveLength(3);
    expect([...highlights].map(line => line.textContent)).toEqual(['# Title', 'line', 'last']);
    expect(scroll).toHaveBeenCalledTimes(1);
    expect(scroll.mock.contexts[0] === highlights[0]).toBe(true);
    expect(scroll.mock.calls[0][0]).toEqual({block: 'center'});
    expect(store.get().session?.files.open['a.md']?.reveal).toBe(false);
    act(() => text('# Changed'));
    act(() => render(null, root));
    mount(store, <FileView id="file:a.md" path="a.md" />);
    expect(scroll).toHaveBeenCalledTimes(1);
  } finally { scroll.mockRestore(); }
});

test('reveal waits for a rendered source highlight, including an empty file', () => {
  const scroll = spyOn(Element.prototype, 'scrollIntoView').mockImplementation(() => {});
  try {
    const session = emptySession('s');
    session.files.open['a.md'] = {...unread, text: '', lines: [{start: 1, end: 1}], reveal: true};
    const store = createStore(testState({session}));
    const root = mount(store, <FileView id="file:a.md" path="a.md" />);
    expect(scroll).not.toHaveBeenCalled();
    expect(store.get().session?.files.open['a.md']?.reveal).toBe(true);
    act(() => store.update(state => setSource(state, 'a.md', true)));
    expect(root.querySelectorAll('.file-view-highlight')).toHaveLength(1);
    expect(scroll).toHaveBeenCalledTimes(1);
    expect(store.get().session?.files.open['a.md']?.reveal).toBe(false);
    act(() => store.update(state => setSource(state, 'a.md', false)));
    expect(root.querySelectorAll('.file-view-highlight')).toHaveLength(0);
  } finally { scroll.mockRestore(); }
});
