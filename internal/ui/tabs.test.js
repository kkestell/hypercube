import {expect, test} from 'bun:test';
import {TabLayout, TabContainer} from './static/tabs.js';

const tabs = layout => layout.panes().map(p => [...p.tabs]);

test('the worker tab area starts empty and opens workers in its first pane', () => {
  const layout = new TabLayout();
  expect(layout.root).toBeNull();
  expect(tabs(layout)).toEqual([]);
  layout.open('one');
  expect(tabs(layout)).toEqual([['one']]);
  layout.open('two');
  expect(tabs(layout)).toEqual([['one', 'two']]);
  expect(layout.find('two').active).toBe('two');
  layout.open('one');
  expect(tabs(layout)).toEqual([['one', 'two']]);
  expect(layout.find('one').active).toBe('one');
});

test('closing the last worker empties the tab area, which can open again', () => {
  const layout = new TabLayout();
  layout.open('one');
  layout.open('two');
  layout.close('two');
  expect(layout.find('one').active).toBe('one');
  layout.close('one');
  expect(layout.root).toBeNull();
  expect(tabs(layout)).toEqual([]);
  layout.close('one');
  layout.open('one');
  expect(tabs(layout)).toEqual([['one']]);
});

test('tabs move between groups, reorder, and collapse their empty source', () => {
  const layout = new TabLayout();
  layout.open('one');
  layout.open('two');
  layout.open('three');
  layout.move('two', layout.find('one'), 'right');
  expect(tabs(layout)).toEqual([['one', 'three'], ['two']]);
  layout.move('three', layout.find('two'), 'center', 'two');
  expect(tabs(layout)).toEqual([['one'], ['three', 'two']]);
  layout.move('one', layout.find('two'), 'center', 'two');
  expect(tabs(layout)).toEqual([['three', 'one', 'two']]);
  expect(layout.root.active).toBe('one');
  layout.move('two', layout.root, 'center', 'three');
  expect(tabs(layout)).toEqual([['two', 'three', 'one']]);
  layout.move('two', layout.root, 'center', 'two');
  expect(tabs(layout)).toEqual([['two', 'three', 'one']]);
});

for (const edge of ['left', 'right', 'top', 'bottom']) {
  test(`dropping on the ${edge} edge creates the corresponding split`, () => {
    const layout = new TabLayout();
    layout.open('one');
    layout.open('two');
    layout.move('two', layout.find('one'), edge);
    const split = layout.root;
    expect(split.axis).toBe(['left', 'right'].includes(edge) ? 'x' : 'y');
    expect(split.children.map(p => p.tabs)).toEqual(['left', 'top'].includes(edge) ? [['two'], ['one']] : [['one'], ['two']]);
    layout.close('two');
    expect(tabs(layout)).toEqual([['one']]);
  });
}

test('a sole tab dropped on its own edges does not create empty panes', () => {
  const layout = new TabLayout();
  layout.open('one');
  for (const edge of ['left', 'right', 'top', 'bottom', 'center']) layout.move('one', layout.root, edge);
  expect(tabs(layout)).toEqual([['one']]);
});

test('new workers use the first pane in a nested layout', () => {
  const layout = new TabLayout();
  layout.open('one');
  layout.open('two');
  layout.open('three');
  layout.move('two', layout.find('one'), 'bottom');
  layout.move('three', layout.find('two'), 'right');
  layout.open('four');
  expect(tabs(layout)).toEqual([['one', 'four'], ['two'], ['three']]);
});

// A small DOM stand-in exercises tab moves without rendering the page.
class Element {
  constructor() {
    this.children = [];
    this.dataset = {};
    this.style = {};
    this.attributes = {};
    this.classList = {toggle() {}};
  }
  get firstElementChild() { return this.children[0]; }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  append(...children) { children.forEach(child => this.insertBefore(child, null)); }
  insertBefore(child, before) {
    child.remove();
    const index = before ? this.children.indexOf(before) : this.children.length;
    this.children.splice(index, 0, child);
    child.parentElement = this;
  }
  remove() {
    if (!this.parentElement) return;
    const siblings = this.parentElement.children;
    siblings.splice(siblings.indexOf(this), 1);
    this.parentElement = null;
  }
  focus() {}
}

test('tab status updates and follows its agent between panes without replacing the transcript', () => {
  const previous = globalThis.document;
  globalThis.document = {createElement: () => new Element()};
  try {
    const layout = new TabLayout();
    layout.open('other');
    layout.open('worker');
    layout.move('worker', layout.find('other'), 'right');
    const root = new Element();
    const container = new TabContainer(root, layout, () => container.render(entries));
    const status = new Element();
    const icon = new Element();
    status.append(icon);
    const other = new Element(), worker = new Element();
    const entries = [{id: 'other', label: 'other', element: other},
      {id: 'worker', label: '<worker>', status, element: worker}];
    container.render(entries);
    const pane = container.nodes.get(layout.find('worker'));
    const tab = pane.bar.firstElementChild.firstElementChild;
    expect(tab.firstElementChild.textContent).toBe('<worker>');
    expect(tab.children[1]).toBe(status);
    expect(tab.attributes['aria-selected']).toBe('true');
    expect(worker.parentElement).toBe(pane.body);

    // State updates preserve the selectable tab and its live status element.
    icon.title = 'Busy';
    container.render(entries);
    expect(pane.bar.firstElementChild.firstElementChild).toBe(tab);
    expect(tab.children[1].firstElementChild.title).toBe('Busy');

    layout.move('worker', layout.find('other'));
    container.render(entries);
    const moved = container.nodes.get(layout.root);
    const movedTab = moved.bar.children[1].firstElementChild;
    expect(movedTab.children[1]).toBe(status);
    expect(worker.parentElement).toBe(moved.body);
    expect(worker.hidden).toBe(false);
    expect(other.hidden).toBe(true);

    entries[1].status = undefined;
    container.render(entries);
    expect(movedTab.children.length).toBe(1);
  } finally {
    globalThis.document = previous;
  }
});

test('worker splits and closing every tab leave the separate lead column intact', () => {
  const previous = globalThis.document;
  globalThis.document = {createElement: () => new Element()};
  try {
    const columns = new Element(), leadColumn = new Element(), tabArea = new Element();
    const lead = new Element();
    lead.value = 'unsent lead draft';
    leadColumn.append(lead);
    columns.append(leadColumn, tabArea);
    const layout = new TabLayout();
    const container = new TabContainer(tabArea, layout, () => {});
    const entries = [{id: 'one', label: 'one', element: new Element()},
      {id: 'two', label: 'two', element: new Element()}];
    container.render(entries);
    expect(tabArea.children.length).toBe(0);
    layout.open('one');
    layout.open('two');
    layout.move('two', layout.find('one'), 'bottom');
    container.render(entries);
    expect(tabs(layout)).toEqual([['one'], ['two']]);
    expect(lead.parentElement).toBe(leadColumn);
    expect(lead.draggable).toBeUndefined();
    expect(lead.value).toBe('unsent lead draft');
    layout.close('one');
    layout.close('two');
    container.render(entries);
    expect(tabArea.children.length).toBe(0);
    expect(container.nodes.size).toBe(0);
    expect(columns.children).toEqual([leadColumn, tabArea]);
    expect(lead.parentElement).toBe(leadColumn);
  } finally {
    globalThis.document = previous;
  }
});
