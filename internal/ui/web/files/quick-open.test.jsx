import {afterEach, expect, spyOn, test} from 'bun:test';
import {act, useState} from 'react';
import {clickControl, mount, setValue, unmountAll} from '../../test-render.jsx';
import {emptySession} from '../sessions/selection.js';
import {createStore} from '../store.js';
import {QuickOpen} from './quick-open.jsx';

afterEach(unmountAll);

test('Quick Open filters session paths and opens a preview', async () => {
  const fetches = spyOn(globalThis, 'fetch').mockImplementation(async url => String(url).startsWith('mention-files?')
    ? Response.json(['src/main.go', 'README.md']) : new Response('source'));
  try {
    const store = createStore({session: emptySession('s'), workspaces: []});
    function Dialog() {
      const [open, setOpen] = useState(true);
      return <QuickOpen open={open} onOpenChange={setOpen} />;
    }
    mount(store, <Dialog />);
    await act(async () => {});
    expect(fetches.mock.calls[0][0]).toContain('session=s');
    const input = document.querySelector('[aria-label="Search files"]');
    act(() => { setValue(input, 'smg'); input.dispatchEvent(new Event('input', {bubbles: true})); });
    expect(document.querySelector('[cmdk-item]').textContent).toBe('src/main.go');
    clickControl(document.querySelector('[cmdk-item]'));
    expect(store.get().session.tabs.preview).toBe('file:src/main.go');
    expect(document.querySelector('[role=dialog]')).toBeNull();
  } finally { fetches.mockRestore(); }
});
