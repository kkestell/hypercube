import {expect, test} from 'bun:test';
import {closeTab, findPane, moveTab, openTab, panes, replaceTab} from './tab-layout.js';

const empty = {root: null, nextId: 1};
const tabs = layout => panes(layout.root).map(pane => pane.tabs);
const open = (...ids) => ids.reduce(openTab, empty);
const pane = (layout, tab) => findPane(layout.root, tab).id;

test('the editor area starts empty and opens tabs in its first pane', () => {
  expect(tabs(empty)).toEqual([]);
  let layout = open('one', 'two');
  expect(tabs(layout)).toEqual([['one', 'two']]);
  expect(findPane(layout.root, 'two').active).toBe('two');
  layout = openTab(layout, 'one');
  expect(tabs(layout)).toEqual([['one', 'two']]);
  expect(findPane(layout.root, 'one').active).toBe('one');
});

test('closing the last tab empties the area, which can open again', () => {
  let layout = closeTab(open('one', 'two'), 'two');
  expect(findPane(layout.root, 'one').active).toBe('one');
  layout = closeTab(layout, 'one');
  expect(layout.root).toBeNull();
  layout = closeTab(layout, 'one');
  expect(tabs(openTab(layout, 'one'))).toEqual([['one']]);
});

test('tabs move between panes, reorder, and collapse their empty source', () => {
  let layout = open('one', 'two', 'three');
  layout = moveTab(layout, 'two', pane(layout, 'one'), 'right');
  expect(tabs(layout)).toEqual([['one', 'three'], ['two']]);
  layout = moveTab(layout, 'three', pane(layout, 'two'), 'center', 'two');
  expect(tabs(layout)).toEqual([['one'], ['three', 'two']]);
  layout = moveTab(layout, 'one', pane(layout, 'two'), 'center', 'two');
  expect(tabs(layout)).toEqual([['three', 'one', 'two']]);
  expect(layout.root.active).toBe('one');
  layout = moveTab(layout, 'two', layout.root.id, 'center', 'three');
  expect(tabs(layout)).toEqual([['two', 'three', 'one']]);
  expect(moveTab(layout, 'two', layout.root.id, 'center', 'two')).toBe(layout);
});

for (const edge of ['left', 'right', 'top', 'bottom']) {
  test(`dropping on the ${edge} edge splits the pane`, () => {
    const layout = moveTab(open('one', 'two'), 'two', 1, edge);
    expect(layout.root.axis).toBe(edge === 'left' || edge === 'right' ? 'x' : 'y');
    expect(layout.root.ratio).toBe(50);
    expect(layout.root.children.map(p => p.tabs)).toEqual(edge === 'left' || edge === 'top' ? [['two'], ['one']] : [['one'], ['two']]);
    const closed = closeTab(layout, 'two');
    expect(tabs(closed)).toEqual([['one']]);
    expect(closed.root.id).toBe(1);
  });
}

test('a sole tab dropped on its own pane changes nothing', () => {
  const layout = open('one');
  for (const edge of ['left', 'right', 'top', 'bottom', 'center']) expect(moveTab(layout, 'one', layout.root.id, edge)).toBe(layout);
});

test('new tabs use the first pane of a nested layout', () => {
  let layout = open('one', 'two', 'three');
  layout = moveTab(layout, 'two', pane(layout, 'one'), 'bottom');
  layout = moveTab(layout, 'three', pane(layout, 'two'), 'right');
  expect(tabs(openTab(layout, 'four'))).toEqual([['one', 'four'], ['two'], ['three']]);
});

test('a preview tab is replaced in place by the next file', () => {
  const layout = replaceTab(open('one', 'preview', 'two'), 'preview', 'next');
  expect(tabs(layout)).toEqual([['one', 'next', 'two']]);
  expect(layout.root.active).toBe('next');
});

test('every function returns a new layout and leaves its input unchanged', () => {
  const layout = moveTab(open('one', 'two', 'three'), 'three', 1, 'right');
  const before = JSON.stringify(layout);
  const results = [
    openTab(layout, 'four'), openTab(layout, 'one'), closeTab(layout, 'two'), replaceTab(layout, 'two', 'five'),
    moveTab(layout, 'one', pane(layout, 'three'), 'center'), moveTab(layout, 'two', pane(layout, 'three'), 'top'),
  ];
  expect(JSON.stringify(layout)).toBe(before);
  for (const result of results) expect(result).not.toBe(layout);
});
