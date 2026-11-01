import {expect, test} from 'bun:test';
import {TabLayout} from './static/tabs.js';

const tabs = layout => layout.panes().map(p => [...p.tabs]);

test('the first worker opens to the right, later workers reuse that pane', () => {
  const layout = new TabLayout('lead');
  layout.open('one');
  expect(layout.root.axis).toBe('x');
  expect(tabs(layout)).toEqual([['lead'], ['one']]);
  layout.open('two');
  expect(tabs(layout)).toEqual([['lead'], ['one', 'two']]);
  expect(layout.find('two').active).toBe('two');
  layout.open('one');
  expect(tabs(layout)).toEqual([['lead'], ['one', 'two']]);
  expect(layout.find('one').active).toBe('one');
});

test('only worker tabs close, and the last worker removes the split', () => {
  const layout = new TabLayout('lead');
  layout.open('one');
  layout.open('two');
  layout.close('lead');
  expect(layout.find('lead')).toBeDefined();
  layout.close('two');
  expect(layout.find('one').active).toBe('one');
  layout.close('one');
  expect(layout.root.tabs).toEqual(['lead']);
  layout.open('one');
  expect(tabs(layout)).toEqual([['lead'], ['one']]);
});

test('tabs move between groups, reorder, and collapse their empty source', () => {
  const layout = new TabLayout('lead');
  layout.open('one');
  layout.open('two');
  layout.move('one', layout.find('lead'));
  expect(tabs(layout)).toEqual([['lead', 'one'], ['two']]);
  layout.move('two', layout.find('lead'), 'center', 'one');
  expect(tabs(layout)).toEqual([['lead', 'two', 'one']]);
  expect(layout.root.active).toBe('two');
  layout.move('one', layout.root, 'center', 'lead');
  expect(tabs(layout)).toEqual([['one', 'lead', 'two']]);
  layout.move('lead', layout.root, 'center', 'lead');
  expect(tabs(layout)).toEqual([['one', 'lead', 'two']]);
});

for (const edge of ['left', 'right', 'top', 'bottom']) {
  test(`dropping on the ${edge} edge creates the corresponding split`, () => {
    const layout = new TabLayout('lead');
    layout.open('one');
    layout.open('two');
    const target = layout.find('one');
    layout.move('two', target, edge);
    const split = layout.root.children[1];
    expect(split.axis).toBe(['left', 'right'].includes(edge) ? 'x' : 'y');
    expect(split.children.map(p => p.tabs)).toEqual(['left', 'top'].includes(edge) ? [['two'], ['one']] : [['one'], ['two']]);
    layout.close('two');
    expect(tabs(layout)).toEqual([['lead'], ['one']]);
  });
}

test('the lead can move and split a group but cannot close', () => {
  const layout = new TabLayout('lead');
  layout.open('one');
  layout.move('lead', layout.find('one'));
  expect(tabs(layout)).toEqual([['one', 'lead']]);
  layout.move('lead', layout.root, 'bottom');
  expect(layout.root.axis).toBe('y');
  expect(tabs(layout)).toEqual([['one'], ['lead']]);
  layout.close('lead');
  expect(tabs(layout)).toEqual([['one'], ['lead']]);
});

test('a sole tab dropped on its own edges does not create empty panes', () => {
  const layout = new TabLayout('lead');
  for (const edge of ['left', 'right', 'top', 'bottom', 'center']) layout.move('lead', layout.root, edge);
  expect(tabs(layout)).toEqual([['lead']]);
});

test('new workers use the first other pane in a nested layout', () => {
  const layout = new TabLayout('lead');
  layout.open('one');
  layout.open('two');
  layout.move('two', layout.find('one'), 'bottom');
  layout.open('three');
  expect(tabs(layout)).toEqual([['lead'], ['one', 'three'], ['two']]);
  layout.move('lead', layout.find('two'), 'bottom');
  layout.open('four');
  expect(tabs(layout)).toEqual([['one', 'three', 'four'], ['two'], ['lead']]);
});
