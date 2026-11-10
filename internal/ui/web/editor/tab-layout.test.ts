import {expect, test} from 'bun:test';
import {closeTab, findPane, mainTabId, moveTab, openTab, panes, replaceTab, withMain} from './tab-layout.ts';
import type {Edge, LayoutNode, TabId, TabLayout} from './types.ts';

const empty: TabLayout = {root: null, preview: null, dragging: null, nextId: 1};
const tabs = (layout: TabLayout) => panes(layout.root).map(pane => pane.tabs);
const open = (...ids: TabId[]) => ids.reduce((layout, id) => openTab(layout, id), empty);
const rooted = (layout: TabLayout): LayoutNode => layout.root ?? expect.unreachable('the layout has no tabs');
const pane = (layout: TabLayout, tab: TabId) => (findPane(layout.root, tab) ?? expect.unreachable(tab)).id;

test('the editor area starts empty and opens tabs in its first pane', () => {
  expect(tabs(empty)).toEqual([]);
  let layout = open('one', 'two');
  expect(tabs(layout)).toEqual([['one', 'two']]);
  expect(findPane(layout.root, 'two')?.active).toBe('two');
  layout = openTab(layout, 'one');
  expect(tabs(layout)).toEqual([['one', 'two']]);
  expect(findPane(layout.root, 'one')?.active).toBe('one');
});

test('closing the last tab empties the area, which can open again', () => {
  let layout = closeTab(open('one', 'two'), 'two');
  expect(findPane(layout.root, 'one')?.active).toBe('one');
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
  expect(rooted(layout)).toMatchObject({active: 'one'});
  layout = moveTab(layout, 'two', rooted(layout).id, 'center', 'three');
  expect(tabs(layout)).toEqual([['two', 'three', 'one']]);
  expect(moveTab(layout, 'two', rooted(layout).id, 'center', 'two')).toBe(layout);
});

const sides: Edge[] = ['left', 'right', 'top', 'bottom'];
for (const edge of sides) {
  test(`dropping on the ${edge} edge splits the pane`, () => {
    const layout = moveTab(open('one', 'two'), 'two', 1, edge);
    expect(rooted(layout)).toMatchObject({axis: edge === 'left' || edge === 'right' ? 'x' : 'y', ratio: 50});
    expect(tabs(layout)).toEqual(edge === 'left' || edge === 'top' ? [['two'], ['one']] : [['one'], ['two']]);
    const closed = closeTab(layout, 'two');
    expect(tabs(closed)).toEqual([['one']]);
    expect(rooted(closed).id).toBe(1);
  });
}

test('a sole tab dropped on its own pane changes nothing', () => {
  const layout = open('one');
  const edges: Edge[] = [...sides, 'center'];
  for (const edge of edges) expect(moveTab(layout, 'one', rooted(layout).id, edge)).toBe(layout);
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
  expect(rooted(layout)).toMatchObject({active: 'next'});
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

test('the first file opens beside the main tab when there is room, and on top of it otherwise', () => {
  const main = withMain(empty);
  expect(tabs(openTab(main, 'file:a'))).toEqual([[mainTabId, 'file:a']]);
  const beside = openTab(main, 'file:a', true);
  const split = rooted(beside);
  expect('axis' in split && split.axis).toBe('x');
  expect(tabs(beside)).toEqual([[mainTabId], ['file:a']]);
  expect(tabs(openTab(beside, 'file:b', true))).toEqual([[mainTabId], ['file:a', 'file:b']]);
  const moved = moveTab(beside, mainTabId, pane(beside, 'file:a'));
  expect(tabs(openTab(moved, 'file:b', true))).toEqual([['file:a', mainTabId, 'file:b']]);
});

test('a layout without the main tab gains it at the left', () => {
  expect(tabs(withMain(empty))).toEqual([[mainTabId]]);
  const files = open('file:a', 'file:b');
  const added = withMain(files);
  expect(tabs(added)).toEqual([[mainTabId], ['file:a', 'file:b']]);
  expect(withMain(added)).toBe(added);
});
