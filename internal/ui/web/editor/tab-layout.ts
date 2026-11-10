// The editor area's tabs form a tree of panes and two-way splits. A pane is
// {id, tabs, active}, and a split is {id, axis, ratio, children: [a, b]}, with
// ratio the first child's percentage. Every function takes a layout
// {root, nextId, ...} and returns a new one, leaving its input unchanged; pane
// and split IDs come from nextId, so they stay stable as the tree changes.
import type {Edge, LayoutNode, Pane, TabId, TabLayout} from './types.ts';

export const panes = (node: LayoutNode | null): Pane[] => !node ? [] : 'tabs' in node ? [node] : node.children.flatMap(panes);

export const findPane = (root: LayoutNode | null, tab: TabId) => panes(root).find(pane => pane.tabs.includes(tab));

// mapNode replaces the node with the given ID by change(node).
function mapNode(node: LayoutNode, id: number, change: (node: LayoutNode) => LayoutNode): LayoutNode {
  if (node.id === id) return change(node);
  if ('tabs' in node) return node;
  const [first, second] = node.children;
  return {...node, children: [mapNode(first, id, change), mapNode(second, id, change)]};
}

const mapPane = (node: LayoutNode, id: number, change: (pane: Pane) => LayoutNode) =>
  mapNode(node, id, found => 'tabs' in found ? change(found) : found);

// prune removes empty panes and splits left with one child.
function prune(node: LayoutNode | null): LayoutNode | null {
  if (!node) return null;
  if ('tabs' in node) return node.tabs.length ? node : null;
  const first = prune(node.children[0]), second = prune(node.children[1]);
  return first && second ? {...node, children: [first, second]} : first ?? second;
}

function withoutTab(pane: Pane, tab: TabId): Pane {
  const index = pane.tabs.indexOf(tab);
  const tabs = pane.tabs.filter(id => id !== tab);
  const active = pane.active === tab ? tabs[Math.min(index, tabs.length - 1)] : pane.active;
  return {...pane, tabs, active};
}

// The main tab is a session's chat or a workspace's dashboard. It moves like
// any tab but never closes.
export const mainTabId: TabId = 'main';

// openTab makes a tab active. A tab that is not open joins the first pane, in
// left-to-right, top-to-bottom order, that holds a tab besides the main tab.
// When the main tab is the only one open, the new tab opens in a pane to its
// right when beside is set, and on top of it otherwise.
export function openTab(layout: TabLayout, tab: TabId, beside = false): TabLayout {
  const {root} = layout;
  if (!root) return {...layout, root: {id: layout.nextId, tabs: [tab], active: tab}, nextId: layout.nextId + 1};
  const all = panes(root);
  const pane = findPane(root, tab) ?? all.find(pane => pane.tabs.some(id => id !== mainTabId)) ?? all[0];
  if (beside && !pane.tabs.includes(tab) && pane.tabs.length === 1 && pane.tabs[0] === mainTabId) {
    const added: Pane = {id: layout.nextId, tabs: [tab], active: tab};
    const split = mapNode(root, pane.id, node => ({id: layout.nextId + 1, axis: 'x', ratio: 50, children: [node, added]}));
    return {...layout, root: split, nextId: layout.nextId + 2};
  }
  const tabs = pane.tabs.includes(tab) ? pane.tabs : [...pane.tabs, tab];
  return {...layout, root: mapNode(root, pane.id, () => ({...pane, tabs, active: tab}))};
}

// withMain adds the main tab to a layout without it, in a pane at the left of
// the tabs already open.
export function withMain(layout: TabLayout): TabLayout {
  const {root, nextId} = layout;
  if (findPane(root, mainTabId)) return layout;
  const main: Pane = {id: nextId, tabs: [mainTabId], active: mainTabId};
  if (!root) return {...layout, root: main, nextId: nextId + 1};
  return {...layout, root: {id: nextId + 1, axis: 'x', ratio: 50, children: [main, root]}, nextId: nextId + 2};
}

export function selectTab(layout: TabLayout, paneId: number, tab: TabId): TabLayout {
  return {...layout, root: layout.root && mapPane(layout.root, paneId, pane => ({...pane, active: tab}))};
}

export function closeTab(layout: TabLayout, tab: TabId): TabLayout {
  const {root} = layout;
  const pane = findPane(root, tab);
  if (!root || !pane) return layout;
  return {...layout, root: prune(mapNode(root, pane.id, () => withoutTab(pane, tab)))};
}

// replaceTab puts a tab in place of an open one, as the next file replaces a
// preview tab.
export function replaceTab(layout: TabLayout, old: TabId, tab: TabId): TabLayout {
  const {root} = layout;
  const pane = findPane(root, old);
  if (!root || !pane) return layout;
  const tabs = pane.tabs.map(id => id === old ? tab : id);
  return {...layout, root: mapNode(root, pane.id, () => ({...pane, tabs, active: tab}))};
}

// moveTab moves a tab into a pane, before another tab or at the end, or splits
// the pane at an edge with the tab in a new pane. Moving a pane's only tab
// within the pane changes nothing.
export function moveTab(layout: TabLayout, tab: TabId, paneId: number, edge: Edge = 'center', before: TabId | null = null): TabLayout {
  const {root} = layout;
  const source = findPane(root, tab);
  if (!root || !source || !panes(root).some(pane => pane.id === paneId)) return layout;
  if (source.id === paneId && (source.tabs.length === 1 || (edge === 'center' && before === tab))) return layout;
  const without = mapNode(root, source.id, () => withoutTab(source, tab));
  if (edge === 'center') {
    const inserted = mapPane(without, paneId, pane => {
      const index = before === null ? -1 : pane.tabs.indexOf(before);
      const tabs = [...pane.tabs];
      tabs.splice(index < 0 ? tabs.length : index, 0, tab);
      return {...pane, tabs, active: tab};
    });
    return {...layout, root: prune(inserted)};
  }
  const added: Pane = {id: layout.nextId, tabs: [tab], active: tab};
  const axis = edge === 'left' || edge === 'right' ? 'x' : 'y';
  const split = mapNode(without, paneId, node => ({
    id: layout.nextId + 1, axis, ratio: 50,
    children: edge === 'left' || edge === 'top' ? [added, node] : [node, added],
  }));
  return {...layout, root: prune(split), nextId: layout.nextId + 2};
}

export function resizeSplit(layout: TabLayout, splitId: number, ratio: number): TabLayout {
  return {...layout, root: layout.root && mapNode(layout.root, splitId, node => 'tabs' in node ? node : {...node, ratio})};
}
