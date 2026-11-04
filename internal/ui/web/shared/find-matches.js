// Find text across inline markup, keeping separate rendered blocks apart.
export function findMatches(list, query) {
  if (!query) return [];
  const walker = document.createTreeWalker(list, NodeFilter.SHOW_TEXT, {acceptNode: node => node.parentElement.closest('[data-find-ignore]') ? NodeFilter.FILTER_REJECT : NodeFilter.FILTER_ACCEPT});
  const blocks = new Map();
  const blockOf = element => {
    if (!element || element === list) return list;
    if (blocks.has(element)) return blocks.get(element);
    const display = getComputedStyle(element).display;
    const block = display && display !== 'inline' && display !== 'contents' ? element : blockOf(element.parentElement);
    blocks.set(element, block);
    return block;
  };
  const nodes = [];
  let text = '', node, block;
  while ((node = walker.nextNode())) {
    const current = blockOf(node.parentElement);
    if (text && current !== block) text += '\n';
    block = current;
    nodes.push({node, start: text.length});
    text += node.data;
  }
  const ranges = [], pattern = new RegExp(query.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), 'gi');
  let first = 0, last = 0;
  for (const match of text.matchAll(pattern)) {
    const start = match.index, end = start + match[0].length;
    while (first + 1 < nodes.length && nodes[first + 1].start <= start) first++;
    if (last < first) last = first;
    while (last + 1 < nodes.length && nodes[last + 1].start < end) last++;
    const range = document.createRange();
    range.setStart(nodes[first].node, start - nodes[first].start);
    range.setEnd(nodes[last].node, end - nodes[last].start);
    ranges.push(range);
  }
  return ranges;
}
