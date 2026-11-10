import {afterEach, beforeEach, expect, mock, test} from 'bun:test';
import {act} from 'react';
import {find, mount, stubFetch, unmountAll} from '../../test-render.tsx';
import {testSession, testState, testWorkspace} from '../../test-state.ts';
import {startViewStorage} from '../app/view-storage.ts';
import {FileView} from '../files/file-view.tsx';
import {unreadFile} from '../files/files.ts';
import {receivedWorkspaces, selectSession} from '../sessions/selection.ts';
import {createStore, useStore} from '../store.ts';
import {Transcript} from '../transcript/transcript.tsx';
import {receivedTranscript} from '../transcript/transcripts.ts';
import {openTab} from '../editor/tab-layout.ts';

const imageComplete = Object.getOwnPropertyDescriptor(HTMLImageElement.prototype, 'complete')!;
// The tests supply file content themselves; reads they start never answer.
beforeEach(() => stubFetch(() => new Promise<Response>(() => {})));
afterEach(() => {
  unmountAll(); mock.restore(); localStorage.clear();
  Object.defineProperty(HTMLImageElement.prototype, 'complete', imageComplete);
});

const workspaces = [testWorkspace({id: 'w', sessions: [testSession({id: 'a'}), testSession({id: 'b'})]})];
const initial = () => receivedWorkspaces(testState({selected: 'a'}), workspaces);

// Model the browser's scroll clamping while data has not produced any content.
function scrollArea(view: HTMLElement) {
  let top = 0, height = 100;
  Object.defineProperties(view, {
    scrollTop: {configurable: true, get: () => top, set: (value: number) => { top = Math.max(0, Math.min(value, height - 100)); }},
    scrollHeight: {configurable: true, get: () => height},
    clientHeight: {configurable: true, get: () => 100},
  });
  return (value: number) => { height = value; };
}

test('file scroll waits for loaded content, saves late unmount, and survives a reload', () => {
  const state = initial();
  state.editor!.tabs = openTab(state.editor!.tabs, 'file:guide.md');
  state.editor!.files.open['guide.md'] = {...unreadFile};
  state.editor!.scroll['file:guide.md'] = {top: 200, left: 12, following: false};
  const store = createStore(state);
  startViewStorage(store);
  function SelectedFile() {
    const [state] = useStore();
    return state.editor?.files.open['guide.md'] ? <FileView key={state.editor.id} id="file:guide.md" path="guide.md" /> : null;
  }
  const root = mount(store, <SelectedFile />);
  let view = find(root, '[aria-label="File contents"]');
  let height = scrollArea(view);
  expect(view.scrollTop).toBe(0);
  const loaded = () => {
    height(1000);
    act(() => store.update(state => ({...state, editor: {...state.editor!, files: {...state.editor!.files, open: {
      ...state.editor!.files.open, 'guide.md': {...state.editor!.files.open['guide.md'], text: 'loaded source'},
    }}}})));
  };
  loaded();
  expect(view.scrollTop).toBe(200);
  expect(view.scrollLeft).toBe(12);
  view.scrollTop = 345;
  act(() => store.update(state => selectSession(state, 'b')));
  expect(store.get().sessionViews.a.scroll['file:guide.md'].top).toBe(345);
  act(() => store.update(state => selectSession(state, 'a')));
  view = find(root, '[aria-label="File contents"]');
  height = scrollArea(view);
  expect(view.scrollTop).toBe(0);
  loaded();
  expect(view.scrollTop).toBe(345);
  view.scrollTop = 456;
  act(() => window.dispatchEvent(new Event('pagehide')));
  const reloaded = createStore(testState());
  startViewStorage(reloaded);
  reloaded.update(state => receivedWorkspaces(state, workspaces));
  expect(reloaded.get().editor!.scroll['file:guide.md'].top).toBe(456);
});

test.each(['load', 'error', 'user scroll'])('Markdown scroll waits for images: %s', outcome => {
  Object.defineProperty(HTMLImageElement.prototype, 'complete', {configurable: true, get: () => false});
  const state = initial();
  state.editor!.tabs = openTab(state.editor!.tabs, 'file:guide.md');
  state.editor!.files.open['guide.md'] = {...unreadFile};
  state.editor!.scroll['file:guide.md'] = {top: 500, left: 0, following: false};
  const store = createStore(state);
  const root = mount(store, <FileView id="file:guide.md" path="guide.md" />);
  const view = find(root, '[aria-label="File contents"]');
  const height = scrollArea(view);
  act(() => store.update(state => ({...state, editor: {...state.editor!, files: {...state.editor!.files, open: {
    ...state.editor!.files.open, 'guide.md': {...unreadFile, text: '![diagram](diagram.png)'},
  }}}})));
  const image = root.querySelector('img')!;
  expect(image.complete).toBe(false);
  expect(view.scrollTop).toBe(0);
  // A browser emits a scroll event after clamping the attempted restoration.
  act(() => {
    view.dispatchEvent(new Event('scroll'));
    window.dispatchEvent(new Event('pagehide'));
  });
  expect(store.get().editor!.scroll['file:guide.md'].top).toBe(500);
  if (outcome === 'user scroll') {
    height(300);
    view.scrollTop = 150;
    act(() => view.dispatchEvent(new Event('scroll')));
  }
  height(1200);
  Object.defineProperty(image, 'complete', {value: true});
  act(() => image.dispatchEvent(new Event(outcome === 'error' ? 'error' : 'load')));
  expect(view.scrollTop).toBe(outcome === 'user scroll' ? 150 : 500);
  act(() => window.dispatchEvent(new Event('pagehide')));
  expect(store.get().editor!.scroll['file:guide.md'].top).toBe(outcome === 'user scroll' ? 150 : 500);
});

test.each([false, true])('transcript restoration keeps following=%s after delayed output and new items', following => {
  const state = initial();
  state.editor!.scroll.a = {top: 200, left: 0, following};
  const store = createStore(state);
  const root = mount(store, <Transcript session={testSession({id: 'a'})} />);
  const view = find(root, '[aria-label=Transcript]').parentElement!;
  const height = scrollArea(view);
  height(1000);
  act(() => store.update(state => receivedTranscript(state, {type: 'transcript', session: 'a', reset: true, items: [{id: 'answer', seq: 1, kind: 'text', text: 'saved answer'}]})));
  expect(view.scrollTop).toBe(following ? 900 : 200);
  act(() => window.dispatchEvent(new Event('pagehide')));
  expect(store.get().editor!.scroll.a.following).toBe(following);
  height(1400);
  act(() => store.update(state => receivedTranscript(state, {type: 'transcript', session: 'a', reset: false, items: [{id: 'new', seq: 2, kind: 'text', text: 'next answer'}]})));
  expect(view.scrollTop).toBe(following ? 1300 : 200);
  act(() => window.dispatchEvent(new Event('pagehide')));
  expect(store.get().editor!.scroll.a.following).toBe(following);
});

test.each([false, true])('transcript preserves image restoration and follow intent; user scroll=%s', userScroll => {
  Object.defineProperty(HTMLImageElement.prototype, 'complete', {configurable: true, get: () => false});
  const state = initial();
  state.editor!.scroll.a = {top: 500, left: 0, following: false};
  const store = createStore(state);
  const root = mount(store, <Transcript session={testSession({id: 'a'})} />);
  const view = find(root, '[aria-label=Transcript]').parentElement!;
  const height = scrollArea(view);
  act(() => store.update(state => receivedTranscript(state, {type: 'transcript', session: 'a', reset: true, items: [
    {id: 'image', seq: 1, kind: 'text', images: ['/transcript-image?index=0']},
  ]})));
  const image = root.querySelector('img')!;
  expect(image.complete).toBe(false);
  expect(view.scrollTop).toBe(0);
  act(() => view.dispatchEvent(new Event('scroll')));
  // Output arriving while the image is incomplete also runs transcript layout.
  act(() => store.update(state => receivedTranscript(state, {type: 'transcript', session: 'a', reset: false, items: [
    {id: 'answer', seq: 2, kind: 'text', text: 'More output'},
  ]})));
  act(() => window.dispatchEvent(new Event('pagehide')));
  expect(store.get().editor!.scroll.a).toEqual({top: 500, left: 0, following: false});
  if (userScroll) {
    height(300);
    view.scrollTop = 150;
    act(() => view.dispatchEvent(new Event('scroll')));
  }
  height(1200);
  Object.defineProperty(image, 'complete', {value: true});
  act(() => image.dispatchEvent(new Event('load')));
  expect(view.scrollTop).toBe(userScroll ? 150 : 500);
  act(() => window.dispatchEvent(new Event('pagehide')));
  expect(store.get().editor!.scroll.a).toEqual({top: userScroll ? 150 : 500, left: 0, following: false});
  height(1400);
  act(() => store.update(state => receivedTranscript(state, {type: 'transcript', session: 'a', reset: false, items: [
    {id: 'next', seq: 3, kind: 'text', text: 'Next answer'},
  ]})));
  expect(view.scrollTop).toBe(userScroll ? 150 : 500);
  act(() => find(root, '[aria-label="Scroll to newest output"]').click());
  expect(view.scrollTop).toBe(1300);
  act(() => window.dispatchEvent(new Event('pagehide')));
  expect(store.get().editor!.scroll.a.following).toBe(true);
});
