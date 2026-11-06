import {afterEach, expect, test} from 'bun:test';
import type {DetachedWindowAPI} from 'happy-dom';
import {act, useRef, useState} from 'react';
import {element, find, mount, setValue, unmountAll} from '../../test-render.tsx';
import {testState} from '../../test-state.ts';
import {emptySession} from '../sessions/selection.ts';
import {createStore, useStore} from '../store.ts';
import {Find} from './find.tsx';

afterEach(unmountAll);

function Pane({id, active, text = 'some text'}: {id: number; active: boolean; text?: string}) {
  const view = useRef<HTMLDivElement>(null), list = useRef<HTMLDivElement>(null);
  return <div className="editor-pane" data-pane={id}>
    <Find view={view} list={list} active={active} label="Find in file" />
    <div ref={view} tabIndex={0}><div ref={list}>{text}</div></div>
  </div>;
}

function key(target: EventTarget, key: string, options: KeyboardEventInit = {}) {
  act(() => target.dispatchEvent(new KeyboardEvent('keydown', {key, bubbles: true, cancelable: true, ...options})));
}

test('find reveals a match beyond the right edge', () => {
  const root = mount(createStore(testState({session: {...emptySession('s'), activePane: 1}})), <Pane id={1} active />);
  const view = find(root, '[tabindex]');
  view.getBoundingClientRect = () => new DOMRect(0, 0, 100, 100);
  const original = Range.prototype.getBoundingClientRect;
  Range.prototype.getBoundingClientRect = () => new DOMRect(150, 0, 30, 10);
  try {
    view.focus();
    key(view, 'f', {metaKey: true});
    const input = element(root, 'input', HTMLInputElement);
    act(() => { setValue(input, 'text'); input.dispatchEvent(new Event('input', {bubbles: true})); });
    expect(view.scrollLeft).toBe(80);
  } finally {
    Range.prototype.getBoundingClientRect = original;
  }
});

test('typing marks the match while focus stays in the field', () => {
  const root = mount(createStore(testState({session: {...emptySession('s'), activePane: 1}})), <Pane id={1} active />);
  key(document.body, 'f', {metaKey: true});
  const input = element(root, 'input', HTMLInputElement);
  act(() => { setValue(input, 't'); input.dispatchEvent(new Event('input', {bubbles: true})); });
  expect([...(CSS.highlights.get('find') ?? [])][0].toString()).toBe('t');
  expect(document.activeElement).toBe(input);
  expect(window.getSelection()?.rangeCount).toBe(0);
});

test('the active pane owns the highlight while another search streams or closes', async () => {
  const store = createStore(testState({session: {...emptySession('s'), activePane: 1}}));
  function SplitView() {
    const [state] = useStore();
    const [text, setText] = useState('some text');
    return <>
      <button onClick={() => setText('some text streamed')}>Stream</button>
      <Pane id={1} active={state.session?.activePane === 1} text={text} />
      <Pane id={2} active={state.session?.activePane === 2} />
    </>;
  }
  const root = mount(store, <SplitView />);
  const query = (id: number, text: string) => {
    key(document.body, 'f', {metaKey: true});
    const input = element(root, `[data-pane="${id}"] input`, HTMLInputElement);
    act(() => { setValue(input, text); input.dispatchEvent(new Event('input', {bubbles: true})); });
    return input;
  };
  const activate = (id: number) => act(() => store.update(state => ({...state, session: {...state.session!, activePane: id}})));
  const highlighted = () => [...(CSS.highlights.get('find') ?? [])].map(range => range.toString());
  query(1, 'text');
  activate(2);
  const input = query(2, 'some');
  expect(highlighted()).toEqual(['some']);
  await act(async () => {
    find(root, 'button').click();
    await (Reflect.get(window, 'happyDOM') as DetachedWindowAPI).whenAsyncComplete();
  });
  expect(document.activeElement).toBe(input);
  expect(highlighted()).toEqual(['some']);
  activate(1);
  expect(highlighted()).toEqual(['text']);
  activate(2);
  act(() => find(root, '[data-pane="1"] [aria-label="Close find"]').click());
  expect(highlighted()).toEqual(['some']);
  act(() => find(root, '[data-pane="2"] [aria-label="Close find"]').click());
  expect(highlighted()).toEqual([]);
});
