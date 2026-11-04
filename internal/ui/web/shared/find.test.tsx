import {afterEach, expect, test} from 'bun:test';
import {act, useRef} from 'react';
import {element, find, mount, setValue, unmountAll} from '../../test-render.tsx';
import {testState} from '../../test-state.ts';
import {createStore} from '../store.ts';
import {Find} from './find.tsx';

afterEach(unmountAll);


function Pane({id, active}: {id: number; active: boolean}) {
  const view = useRef<HTMLDivElement>(null), list = useRef<HTMLDivElement>(null);
  return <div className="editor-pane" data-pane={id}>
    <Find view={view} list={list} active={active} label="Find in file" />
    <div ref={view} tabIndex={0}><div ref={list}>some text</div></div>
  </div>;
}

function key(target: EventTarget, key: string, options: KeyboardEventInit = {}) {
  act(() => target.dispatchEvent(new KeyboardEvent('keydown', {key, bubbles: true, cancelable: true, ...options})));
}

test('body-focused find opens only the last used pane in a split', () => {
  const store = createStore(testState({workbench: {...testState().workbench, activePane: 2}}));
  const root = mount(store, <><Pane id={1} active={false} /><Pane id={2} active /></>);
  key(document.body, 'f', {metaKey: true});
  expect(root.querySelectorAll('[role=search]')).toHaveLength(1);
  expect(root.querySelector('[data-pane="2"] [role=search]')).not.toBeNull();
});

test('find reveals a match beyond the right edge', () => {
  const root = mount(createStore(testState({workbench: {...testState().workbench, activePane: 1}})), <Pane id={1} active />);
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
  const root = mount(createStore(testState({workbench: {...testState().workbench, activePane: 1}})), <Pane id={1} active />);
  key(document.body, 'f', {metaKey: true});
  const input = element(root, 'input', HTMLInputElement);
  act(() => { setValue(input, 't'); input.dispatchEvent(new Event('input', {bubbles: true})); });
  expect([...(CSS.highlights.get('find') ?? [])][0].toString()).toBe('t');
  expect(document.activeElement).toBe(input);
  expect(window.getSelection()?.rangeCount).toBe(0);
});
