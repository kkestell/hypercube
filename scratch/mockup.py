#!/usr/bin/env python3
"""Print a mockup of Hypercube's screen: the current layout and styles.

Mirrors host.rs (layout, TeamChrome, draw_label) and bar.rs. Pane contents are
blank. A thin gray border drawn outside the canvas shows where it ends.

    ./mockup.py --cols 100 --rows 30 --workers 2 --focus 1
"""
import argparse

RESET = "\x1b[0m"
DIVIDER = "\x1b[0;38;2;70;160;220m"
TITLE_TEXT = "\x1b[0;38;2;144;198;234m"
LABEL = "\x1b[0;38;2;227;241;250;48;2;15;35;48m"
BAR = LABEL
HEADER = LABEL
HARNESS = "\x1b[0;48;2;40;46;64m"
BOLD = "\x1b[1m"
FRAME = "\x1b[0;38;2;100;100;100m"


class Canvas:
    """A grid of cells; each is (char, sgr)."""

    def __init__(self, cols, rows):
        self.cols, self.rows = cols, rows
        self.cells = [[(" ", RESET)] * cols for _ in range(rows)]

    def put(self, row, col, text, sgr):
        for i, ch in enumerate(text):
            if 0 <= row < self.rows and 0 <= col + i < self.cols:
                self.cells[row][col + i] = (ch, sgr)

    def render(self, border):
        lines = []
        if border:
            lines.append(f"{FRAME}┌{'─' * self.cols}┐{RESET}")
        for row in self.cells:
            out, prev = [], None
            for ch, sgr in row:
                if sgr != prev:
                    out.append(sgr)
                    prev = sgr
                out.append(ch)
            body = "".join(out) + RESET
            lines.append(f"{FRAME}│{RESET}{body}{FRAME}│{RESET}" if border else body)
        if border:
            lines.append(f"{FRAME}└{'─' * self.cols}┘{RESET}")
        return "\n".join(lines)


def draw_label(c, row, col, width, name, detail, focused):
    """A title drawn on a horizontal line: ━ name ━━━━━━ detail ━"""
    c.put(row, col, "━" * width, DIVIDER)
    sgr = TITLE_TEXT + (BOLD if focused else "")
    left = f" {name} "[: max(width - 1, 0)]
    c.put(row, col + 1, left, sgr)
    right = f" {detail} "
    if detail and 1 + len(left) + 1 + len(right) + 1 <= width:
        c.put(row, col + width - 1 - len(right), right, sgr)


def draw(args):
    cols, rows = args.cols, args.rows
    c = Canvas(cols, rows)
    # The header takes the first row and the bar the last; the panes go between.
    y0, area = 1, rows - 2
    # The orchestrator has no label; the header names its workspace.
    panes: list[tuple[str | None, str]] = [(None, "")]
    for i in range(1, args.workers + 1):
        panes.append((f"worker-{i}", f"⎇ feature-{i}"))
    # No gaps, and a divider between the orchestrator and the workers.
    n = len(panes) - 1
    left = cols if n == 0 else cols * 2 // 5
    rects = [(0, y0, left, area)]
    for i in range(n):
        top, bottom = area * i // n, area * (i + 1) // n
        rects.append((left + 1, y0 + top, cols - left - 1, bottom - top))

    c.put(0, 0, " " * cols, HEADER)
    c.put(0, 0, f" {args.path}"[:cols], HEADER)
    if n:
        for row in range(y0, y0 + area):
            c.put(row, left, "┃", DIVIDER)
    for i, ((name, detail), (col, top, width, height)) in enumerate(zip(panes, rects)):
        for row in range(top if name is None else top + 1, top + height):
            c.put(row, col, " " * width, HARNESS)
        if name is not None:
            draw_label(c, top, col, width, name, detail, i == args.focus)
            # The divider branches into the title's line.
            c.put(top, left, "┣", DIVIDER)

    bar = [("hypercube", True), ("api-server", False), ("docs", False)]
    c.put(rows - 1, 0, " " * cols, BAR)
    used, col = 0, 0
    for i, (name, current) in enumerate(bar):
        gap = " " if i == 0 else "  "
        text = f"{i} {'*' if current else ''}{name}"
        used += len(gap) + len(text)
        if used > cols:
            break
        c.put(rows - 1, col + len(gap), text, BAR + (BOLD if current else ""))
        col += len(gap) + len(text)
    return c.render(not args.no_border)


def main():
    p = argparse.ArgumentParser(description=(__doc__ or "").split("\n")[0])
    p.add_argument("--cols", type=int, default=100)
    p.add_argument("--rows", type=int, default=30)
    p.add_argument("--workers", type=int, default=2)
    p.add_argument("--focus", type=int, default=0, help="focused pane index; 0 is the orchestrator")
    p.add_argument("--path", default="~/projects/hypercube", help="workspace path for the header")
    p.add_argument("--no-border", action="store_true", help="omit the gray canvas border")
    print(draw(p.parse_args()))


if __name__ == "__main__":
    main()
