import {afterEach, expect, test} from 'bun:test';
import {act, useRef} from 'react';
import {mount, setValue, unmountAll} from '../../test-render.jsx';
import {createStore} from '../store.js';
import {Find} from './find.jsx';

afterEach(unmountAll);

function Pane({id, active}) {
  const view = useRef(null), list = useRef(null);
  return <div className="editor-pane" data-pane={id}>
    <Find view={view} list={list} active={active} label="Find in file" />
    <div ref={view} tabIndex={0}><div ref={list}>some text</div></div>
  </div>;
}

function key(target, key, options = {}) {
  act(() => target.dispatchEvent(new KeyboardEvent('keydown', {key, bubbles: true, cancelable: true, ...options})));
}

test('body-focused find opens only the last used pane in a split', () => {
  const store = createStore({workbench: {activePane: 2}});
  const root = mount(store, <><Pane id={1} active={false} /><Pane id={2} active /></>);
  key(document.body, 'f', {metaKey: true});
  expect(root.querySelectorAll('[role=search]')).toHaveLength(1);
  expect(root.querySelector('[data-pane="2"] [role=search]')).not.toBeNull();
});

test('find reveals a match beyond the right edge', () => {
  const root = mount(createStore({workbench: {activePane: 1}}), <Pane id={1} active />);
  const view = root.querySelector('[tabindex]');
  view.getBoundingClientRect = () => ({top: 0, bottom: 100, left: 0, right: 100, height: 100});
  const original = Range.prototype.getBoundingClientRect;
  Range.prototype.getBoundingClientRect = () => ({top: 0, bottom: 10, left: 150, right: 180});
  try {
    view.focus();
    key(view, 'f', {metaKey: true});
    const input = root.querySelector('input');
    act(() => { setValue(input, 'text'); input.dispatchEvent(new Event('input', {bubbles: true})); });
    expect(view.scrollLeft).toBe(80);
  } finally {
    Range.prototype.getBoundingClientRect = original;
  }
});

test('typing marks the match while focus stays in the field', () => {
  const root = mount(createStore({workbench: {activePane: 1}}), <Pane id={1} active />);
  key(document.body, 'f', {metaKey: true});
  const input = root.querySelector('input');
  act(() => { setValue(input, 't'); input.dispatchEvent(new Event('input', {bubbles: true})); });
  expect(CSS.highlights.get('find').ranges[0].toString()).toBe('t');
  expect(document.activeElement).toBe(input);
  expect(window.getSelection().rangeCount).toBe(0);
});
