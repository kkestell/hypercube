#!/usr/bin/env python3
"""Print a mockup of Hypercube's screen: the current layout and styles.

Mirrors host.rs (layout, TeamChrome, draw_label) and bar.rs. Pane contents are
blank. A thin gray border drawn outside the canvas shows where it ends.

    ./mockup.py --cols 100 --rows 30 --workers 2 --focus 1
"""
import argparse

RESET = "\x1b[0m"
BOLD = "\x1b[1m"
FRAME = "\x1b[0;38;2;100;100;100m"
HARNESS = "\x1b[0;48;2;40;46;64m"
ATTENTION = (255, 70, 70)
# Mirrors colors.rs.
ACCENTS = [
    (70, 160, 220),
    (40, 180, 170),
    (100, 180, 80),
    (220, 170, 50),
    (230, 120, 50),
    (220, 80, 80),
    (220, 90, 170),
    (150, 110, 220),
]


def mix(c, to, amount):
    return tuple(int(a + (b - a) * amount + 0.5) for a, b in zip(c, to))


def fg(c):
    return "\x1b[0;38;2;{};{};{}m".format(*c)


def sgr(fg, bg):
    return "\x1b[0;38;2;{};{};{};48;2;{};{};{}m".format(*fg, *bg)


class Palette:
    """The chrome's colors, all shades of the accent numbered `n`."""

    def __init__(self, n):
        accent = ACCENTS[n % len(ACCENTS)]
        white, black = (255, 255, 255), (0, 0, 0)
        self.line = accent
        self.text = mix(accent, white, 0.4)
        self.bar_fg, self.bar_bg = mix(accent, white, 0.85), mix(accent, black, 0.78)


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


def draw_label(c, pal, row, col, width, name, detail, focused, attention):
    """A title drawn on a horizontal line: ━ name ━━━━━━ detail ━, with a red
    dot after the name while the agent waits for an approval."""
    c.put(row, col, "━" * width, fg(pal.line))
    style = fg(pal.text) + (BOLD if focused else "")
    dot = " ●" if attention else ""
    left = f" {name}{dot} "[: max(width - 1, 0)]
    c.put(row, col + 1, left, style)
    if attention and len(left) >= len(name) + 3:
        c.put(row, col + 2 + len(name), dot, fg(ATTENTION) + (BOLD if focused else ""))
    right = f" {detail} "
    if detail and 1 + len(left) + 1 + len(right) + 1 <= width:
        c.put(row, col + width - 1 - len(right), right, style)


def draw(args):
    cols, rows = args.cols, args.rows
    c = Canvas(cols, rows)
    pal = Palette(args.accent)
    divider, bar = fg(pal.line), sgr(pal.bar_fg, pal.bar_bg)
    # The header takes the first row and the bar the last; the panes go between.
    y0, area = 1, rows - 2
    panes = [("orchestrator", "⎇ main", False)]
    for i in range(1, args.workers + 1):
        panes.append((f"worker-{i}", f"⎇ feature-{i}", i == 2))
    # No gaps, and a divider splitting the width evenly between the
    # orchestrator and the workers.
    n = len(panes) - 1
    left = cols if n == 0 else (cols - 1) // 2
    rects = [(0, y0, left, area)]
    for i in range(n):
        top, bottom = area * i // n, area * (i + 1) // n
        rects.append((left + 1, y0 + top, cols - left - 1, bottom - top))

    # The header names the workspace at the left and the orchestrator's notes
    # at the right, in the bar colors.
    c.put(0, 0, " " * cols, bar)
    c.put(0, 0, f" {args.path}"[:cols], bar)
    if args.notes and len(args.path) + len(args.notes) + 3 <= cols:
        c.put(0, cols - len(args.notes) - 1, args.notes, bar)
    if n:
        for row in range(y0, y0 + area):
            c.put(row, left, "┃", divider)
    for i, ((name, detail, attention), (col, top, width, height)) in enumerate(zip(panes, rects)):
        for row in range(top + 1, top + height):
            c.put(row, col, " " * width, HARNESS)
        draw_label(c, pal, top, col, width, name, detail, i == args.focus, attention)
        if i:
            # The divider branches into the title's line, and the top titles meet it.
            c.put(top, left, "┳" if top == y0 else "┣", divider)

    # The bar: a tab for each workspace that fits, in the bar colors, with a
    # dim separator between them, the current one bold and the others dim, and a red dot after any
    # workspace waiting for an approval. The current workspace is the first.
    # The current workspace has a dot while any of its agents waits.
    waiting = any(attention for _, _, attention in panes)
    tabs = [("hypercube", waiting), ("api-server", True), ("docs", False)]
    c.put(rows - 1, 0, " " * cols, bar)
    col = 1
    for i, (name, attention) in enumerate(tabs):
        bold = BOLD if i == 0 else ""
        sep = " | " if i else ""
        dot = " ●" if attention else ""
        text = f"{i} {name}"
        if col + len(sep) + len(text) + len(dot) > cols:
            break
        c.put(rows - 1, col, sep, sgr(mix(pal.bar_fg, pal.bar_bg, 0.55), pal.bar_bg))
        col += len(sep)
        dim = sgr(mix(pal.bar_fg, pal.bar_bg, 0.4), pal.bar_bg)
        c.put(rows - 1, col, text, bar + bold if i == 0 else dim)
        col += len(text)
        if attention:
            c.put(rows - 1, col, dot, sgr(ATTENTION, pal.bar_bg) + bold)
            col += len(dot)
    return c.render(not args.no_border)


def main():
    p = argparse.ArgumentParser(description=(__doc__ or "").split("\n")[0])
    p.add_argument("--cols", type=int, default=100)
    p.add_argument("--rows", type=int, default=30)
    p.add_argument("--workers", type=int, default=2)
    p.add_argument("--focus", type=int, default=0, help="focused pane index; 0 is the orchestrator")
    p.add_argument("--path", default="~/projects/hypercube", help="workspace path for the header")
    p.add_argument("--accent", type=int, default=0, help="the workspace's accent, numbered in ACCENTS")
    p.add_argument("--notes", default="", help="the orchestrator's notes for the header, e.g. 'zoomed'")
    p.add_argument("--no-border", action="store_true", help="omit the gray canvas border")
    print(draw(p.parse_args()))


if __name__ == "__main__":
    main()
