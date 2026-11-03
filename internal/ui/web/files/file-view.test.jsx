import {afterEach, expect, spyOn, test} from 'bun:test';
import {render} from 'preact';
import {act} from 'preact/test-utils';
import {mount, unmountAll} from '../../test-render.jsx';
import {setSource} from './files.js';
import {emptySession} from '../sessions/selection.js';
import {createStore, StoreContext} from '../store.js';
import {FileView} from './file-view.jsx';

afterEach(unmountAll);

test('Markdown file previews render images relative to the file and session', () => {
  const path = 'docs #1/guide.md';
  const session = emptySession('s & 1');
  session.files.open[path] = {message: null, text: `
- ![Local](images/picture.png "Caption")

| Image |
|---|
| ![Parent](../assets/picture%20%23.png) |

![Root](/assets/root.png)
![SVG](drawing.svg#part)
![Remote](https://example.com/picture.png?size=2)
![Blocked](javascript:alert%281%29)
![File](file:///etc/passwd)
![HTML](data:text/html,hello)
<img src=x onerror=alert(1)>
`};
  const store = createStore({session, workspaces: []});
  const root = document.createElement('div');
  try {
    render(<StoreContext.Provider value={[store.get(), store]}><FileView id="file" path={path} /></StoreContext.Provider>, root);
    const images = [...root.querySelectorAll('img')];
    expect(images.map(image => image.alt)).toEqual(['Local', 'Parent', 'Root', 'SVG', 'Remote']);
    const sources = images.map(image => new URL(image.getAttribute('src'), 'http://localhost:4280'));
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

test('source highlights reveal once after text arrives, across refreshes and remounts', () => {
  const scroll = spyOn(Element.prototype, 'scrollIntoView').mockImplementation(() => {});
  try {
    const session = emptySession('s');
    session.files.open['a.md'] = {message: null, text: null, source: true, lines: [{start: 1, end: 2}, {start: 3, end: 3}], reveal: true};
    session.scroll['file:a.md'] = {top: 100, left: 0, following: false};
    const store = createStore({session, workspaces: []});
    const root = mount(store, <FileView id="file:a.md" path="a.md" />);
    expect(root.querySelectorAll('.file-view-highlight')).toHaveLength(0);
    expect(scroll).not.toHaveBeenCalled();
    const text = value => store.update(state => ({...state, session: {...state.session, files: {...state.session.files, open: {
      'a.md': {...state.session.files.open['a.md'], text: value},
    }}}}));
    act(() => text('# Title\nline\nlast'));
    expect(root.querySelector('.markdown-preview')).toBeNull();
    const highlights = root.querySelectorAll('.file-view-highlight');
    expect(highlights).toHaveLength(2);
    expect(highlights[0].style.getPropertyValue('--start')).toBe('1');
    expect(highlights[0].style.getPropertyValue('--count')).toBe('2');
    expect(scroll).toHaveBeenCalledTimes(1);
    expect(scroll.mock.contexts[0] === highlights[0]).toBe(true);
    expect(scroll.mock.calls[0][0]).toEqual({block: 'center'});
    expect(store.get().session.files.open['a.md'].reveal).toBe(false);
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
    session.files.open['a.md'] = {message: null, text: '', source: false, lines: [{start: 1, end: 1}], reveal: true};
    const store = createStore({session, workspaces: []});
    const root = mount(store, <FileView id="file:a.md" path="a.md" />);
    expect(scroll).not.toHaveBeenCalled();
    expect(store.get().session.files.open['a.md'].reveal).toBe(true);
    act(() => store.update(state => setSource(state, 'a.md', true)));
    expect(root.querySelectorAll('.file-view-highlight')).toHaveLength(1);
    expect(scroll).toHaveBeenCalledTimes(1);
    expect(store.get().session.files.open['a.md'].reveal).toBe(false);
    act(() => store.update(state => setSource(state, 'a.md', false)));
    expect(root.querySelectorAll('.file-view-highlight')).toHaveLength(0);
  } finally { scroll.mockRestore(); }
});
