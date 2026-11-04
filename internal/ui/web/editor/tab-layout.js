// The editor area's tabs form a tree of panes and two-way splits. A pane is
// {id, tabs, active}, and a split is {id, axis, ratio, children: [a, b]}, with
// ratio the first child's percentage. Every function takes a layout
// {root, nextId, ...} and returns a new one, leaving its input unchanged; pane
// and split IDs come from nextId, so they stay stable as the tree changes.

export const panes = node => !node ? [] : node.tabs ? [node] : node.children.flatMap(panes);

export const findPane = (root, tab) => panes(root).find(pane => pane.tabs.includes(tab));

// mapNode replaces the node with the given ID by change(node).
function mapNode(node, id, change) {
  if (node.id === id) return change(node);
  if (node.tabs) return node;
  return {...node, children: node.children.map(child => mapNode(child, id, change))};
}

// prune removes empty panes and splits left with one child.
function prune(node) {
  if (!node) return null;
  if (node.tabs) return node.tabs.length ? node : null;
  const children = node.children.map(prune).filter(Boolean);
  if (children.length < 2) return children[0] ?? null;
  return {...node, children};
}

function withoutTab(pane, tab) {
  const index = pane.tabs.indexOf(tab);
  const tabs = pane.tabs.filter(id => id !== tab);
  const active = pane.active === tab ? tabs[Math.min(index, tabs.length - 1)] : pane.active;
  return {...pane, tabs, active};
}

// openTab makes a tab active, adding it to the first pane, in left-to-right,
// top-to-bottom order, if it is not open.
export function openTab(layout, tab) {
  const pane = findPane(layout.root, tab) ?? panes(layout.root)[0];
  if (!pane) return {...layout, root: {id: layout.nextId, tabs: [tab], active: tab}, nextId: layout.nextId + 1};
  const tabs = pane.tabs.includes(tab) ? pane.tabs : [...pane.tabs, tab];
  return {...layout, root: mapNode(layout.root, pane.id, () => ({...pane, tabs, active: tab}))};
}

export function selectTab(layout, paneId, tab) {
  return {...layout, root: mapNode(layout.root, paneId, pane => ({...pane, active: tab}))};
}

export function closeTab(layout, tab) {
  const pane = findPane(layout.root, tab);
  if (!pane) return layout;
  return {...layout, root: prune(mapNode(layout.root, pane.id, () => withoutTab(pane, tab)))};
}

// replaceTab puts a tab in place of an open one, as the next file replaces a
// preview tab.
export function replaceTab(layout, old, tab) {
  const pane = findPane(layout.root, old);
  const tabs = pane.tabs.map(id => id === old ? tab : id);
  return {...layout, root: mapNode(layout.root, pane.id, () => ({...pane, tabs, active: tab}))};
}

// moveTab moves a tab into a pane, before another tab or at the end, or splits
// the pane at an edge with the tab in a new pane. Moving a pane's only tab
// within the pane changes nothing.
export function moveTab(layout, tab, paneId, edge = 'center', before = null) {
  const source = findPane(layout.root, tab);
  if (!source || !panes(layout.root).some(pane => pane.id === paneId)) return layout;
  if (source.id === paneId && (source.tabs.length === 1 || (edge === 'center' && before === tab))) return layout;
  const root = mapNode(layout.root, source.id, () => withoutTab(source, tab));
  if (edge === 'center') {
    const inserted = mapNode(root, paneId, pane => {
      const index = pane.tabs.indexOf(before);
      const tabs = [...pane.tabs];
      tabs.splice(index < 0 ? tabs.length : index, 0, tab);
      return {...pane, tabs, active: tab};
    });
    return {...layout, root: prune(inserted)};
  }
  const added = {id: layout.nextId, tabs: [tab], active: tab};
  const axis = edge === 'left' || edge === 'right' ? 'x' : 'y';
  const split = mapNode(root, paneId, pane => ({
    id: layout.nextId + 1, axis, ratio: 50,
    children: edge === 'left' || edge === 'top' ? [added, pane] : [pane, added],
  }));
  return {...layout, root: prune(split), nextId: layout.nextId + 2};
}

export function resizeSplit(layout, splitId, ratio) {
  return {...layout, root: mapNode(layout.root, splitId, split => ({...split, ratio}))};
}
