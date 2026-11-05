import {afterEach, expect, mock, test} from 'bun:test';
import {act, useState} from 'react';
import {clickControl, element, find, mount, setValue, stubFetch, unmountAll} from '../../test-render.tsx';
import {testState} from '../../test-state.ts';
import {emptySession} from '../sessions/selection.ts';
import {createStore} from '../store.ts';
import {QuickOpen} from './quick-open.tsx';

afterEach(() => { unmountAll(); mock.restore(); });

test('Quick Open filters session paths and opens a preview', async () => {
  const fetches = stubFetch(url => url.startsWith('mention-files?')
    ? Response.json(['src/main.go', 'README.md']) : new Response('source'));
  const store = createStore(testState({session: emptySession('s')}));
  function Dialog() {
    const [open, setOpen] = useState(true);
    return <QuickOpen open={open} onOpenChange={setOpen} />;
  }
  mount(store, <Dialog />);
  await act(async () => {});
  expect(String(fetches.mock.calls[0]?.[0])).toContain('session=s');
  const input = element(document, '[aria-label="Search files"]', HTMLInputElement);
  act(() => { setValue(input, 'smg'); input.dispatchEvent(new Event('input', {bubbles: true})); });
  expect(find(document, '[cmdk-item]').textContent).toBe('src/main.go');
  await act(async () => { clickControl(find(document, '[cmdk-item]')); });
  expect(store.get().session?.tabs.preview).toBe('file:src/main.go');
  expect(document.querySelector('[role=dialog]')).toBeNull();
});

test('Escape and selection restore previous focus', async () => {
  stubFetch(url => url.startsWith('mention-files?')
    ? Response.json(['src/main.go']) : new Response('source'));
  const store = createStore(testState({session: emptySession('s')}));
  function View() {
    const [open, setOpen] = useState(false);
    return <><button onClick={() => setOpen(true)}>Open picker</button><QuickOpen open={open} onOpenChange={setOpen} /></>;
  }
  const root = mount(store, <View />);
  const button = find(root, 'button');
  act(() => button.focus());
  clickControl(button);
  await act(async () => {});
  act(() => find(document, '[aria-label="Search files"]').dispatchEvent(new KeyboardEvent('keydown', {key: 'Escape', bubbles: true, cancelable: true})));
  await act(async () => new Promise(resolve => setTimeout(resolve, 0)));
  expect(document.activeElement).toBe(button);
  clickControl(button);
  await act(async () => {});
  clickControl(find(document, '[cmdk-item]'));
  await act(async () => new Promise(resolve => setTimeout(resolve, 0)));
  expect(document.activeElement).toBe(button);
});

test('Command+P and Command+Shift+P prevent printing without changing selection', async () => {
  stubFetch(() => Response.json(['alpha.go', 'beta.go', 'gamma.txt']));
  const store = createStore(testState({session: emptySession('s')}));
  mount(store, <QuickOpen open onOpenChange={() => {}} />);
  await act(async () => {});
  const input = element(document, '[aria-label="Search files"]', HTMLInputElement);
  act(() => { setValue(input, '.go'); input.dispatchEvent(new Event('input', {bubbles: true})); });
  const selected = () => document.querySelector('[cmdk-item][aria-selected="true"]')?.textContent;
  expect(selected()).toBe('alpha.go');
  const press = (shiftKey: boolean) => {
    const event = new KeyboardEvent('keydown', {key: 'p', metaKey: true, shiftKey, bubbles: true, cancelable: true});
    act(() => input.dispatchEvent(event));
    expect(event.defaultPrevented).toBe(true);
  };
  press(false);
  expect(selected()).toBe('alpha.go');
  press(true);
  expect(selected()).toBe('alpha.go');
});
