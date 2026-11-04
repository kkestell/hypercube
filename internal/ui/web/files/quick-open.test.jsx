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
    const store = createStore({session: emptySession('s'), workspaces: [], overlays: {}});
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

test('Escape and selection restore previous focus', async () => {
  const fetches = spyOn(globalThis, 'fetch').mockImplementation(async url => String(url).startsWith('mention-files?')
    ? Response.json(['src/main.go']) : new Response('source'));
  try {
    const store = createStore({session: emptySession('s'), workspaces: [], overlays: {}});
    function View() {
      const [open, setOpen] = useState(false);
      return <><button onClick={() => setOpen(true)}>Open picker</button><QuickOpen open={open} onOpenChange={setOpen} /></>;
    }
    const root = mount(store, <View />);
    const button = root.querySelector('button');
    act(() => button.focus());
    clickControl(button);
    await act(async () => {});
    act(() => document.querySelector('[aria-label="Search files"]').dispatchEvent(new KeyboardEvent('keydown', {key: 'Escape', bubbles: true, cancelable: true})));
    await act(async () => new Promise(resolve => setTimeout(resolve, 0)));
    expect(document.activeElement).toBe(button);
    clickControl(button);
    await act(async () => {});
    clickControl(document.querySelector('[cmdk-item]'));
    await act(async () => new Promise(resolve => setTimeout(resolve, 0)));
    expect(document.activeElement).toBe(button);
  } finally { fetches.mockRestore(); }
});

test('Command+P and Command+Shift+P prevent printing without changing selection', async () => {
  const fetches = spyOn(globalThis, 'fetch').mockImplementation(async () => Response.json(['alpha.go', 'beta.go', 'gamma.txt']));
  try {
    const store = createStore({session: emptySession('s'), workspaces: [], overlays: {}});
    mount(store, <QuickOpen open onOpenChange={() => {}} />);
    await act(async () => {});
    const input = document.querySelector('[aria-label="Search files"]');
    act(() => { setValue(input, '.go'); input.dispatchEvent(new Event('input', {bubbles: true})); });
    const selected = () => document.querySelector('[cmdk-item][aria-selected="true"]')?.textContent;
    expect(selected()).toBe('alpha.go');
    const press = shiftKey => {
      const event = new KeyboardEvent('keydown', {key: 'p', metaKey: true, shiftKey, bubbles: true, cancelable: true});
      act(() => input.dispatchEvent(event));
      expect(event.defaultPrevented).toBe(true);
    };
    press(false);
    expect(selected()).toBe('alpha.go');
    press(true);
    expect(selected()).toBe('alpha.go');
  } finally { fetches.mockRestore(); }
});
