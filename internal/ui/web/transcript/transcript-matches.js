// Find text across inline markup, keeping separate rendered blocks apart.
export function transcriptMatches(list, query) {
  if (!query) return [];
  const walker = document.createTreeWalker(list, NodeFilter.SHOW_TEXT);
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
  const ranges = [], needle = query.toLowerCase(), haystack = text.toLowerCase();
  let first = 0, last = 0;
  for (let start = 0; (start = haystack.indexOf(needle, start)) !== -1; start += needle.length) {
    const end = start + needle.length;
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
