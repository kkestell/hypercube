import {expect, test} from 'bun:test';
import {render} from 'preact';
import {emptySession} from '../sessions/selection.js';
import {createStore, StoreContext} from '../store.js';
import {FileView} from './file-view.jsx';

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
