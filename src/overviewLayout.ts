export type Size = { width: number; height: number };

/** Where a workspace card sits: its top-left corner and its scale relative to the window. */
export type CardPlacement = { x: number; y: number; scale: number };

const PADDING = 48;
const GAP = 32;

/**
 * Arranges `count` cards, each a scaled-down copy of the window, in the grid
 * that makes them largest. Rows are centered, including a shorter last row.
 */
export function layoutCards(count: number, viewport: Size, bottomReserve = 0): CardPlacement[] {
  const { width, height } = viewport;
  /** Height the grid may use; cards keep the window's aspect ratio. */
  const available = height - bottomReserve;

  let columns = 1;
  let scale = 0;
  for (let candidate = 1; candidate <= count; candidate++) {
    const rows = Math.ceil(count / candidate);
    const candidateScale = Math.min(
      (width - 2 * PADDING - (candidate - 1) * GAP) / candidate / width,
      (available - 2 * PADDING - (rows - 1) * GAP) / rows / height,
    );
    if (candidateScale > scale) {
      columns = candidate;
      scale = candidateScale;
    }
  }
  scale = Math.max(scale, 0.05);

  const rows = Math.ceil(count / columns);
  const cardWidth = width * scale;
  const cardHeight = height * scale;
  const top = (available - (rows * cardHeight + (rows - 1) * GAP)) / 2;

  return Array.from({ length: count }, (_, index) => {
    const row = Math.floor(index / columns);
    const column = index % columns;
    const cardsInRow = Math.min(columns, count - row * columns);
    const left = (width - (cardsInRow * cardWidth + (cardsInRow - 1) * GAP)) / 2;
    return { x: left + column * (cardWidth + GAP), y: top + row * (cardHeight + GAP), scale };
  });
}
