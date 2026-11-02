// Normalize every expanded card to its rendered content height before a drag.
// Flex then preserves the heights of cards outside the resized pair.
export function resizeWorkers(heights, before, after, delta, minimum = 120) {
  const total = heights[before] + heights[after];
  const bound = Math.min(minimum, total / 2);
  const first = Math.max(bound, Math.min(total - bound, heights[before] + delta));
  return {...heights, [before]: first, [after]: total - first};
}
