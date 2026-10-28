# Themes

The direction for how Hypercube looks. Only the first theme exists; the rest is
a plan, and it may change.

## Direction

A theme is a built-in layout and border style. Two or three will ship:

- **Fancy**: the one that exists. A header and a footer, a gap around and
  between the panes, a backdrop behind them, and a drop shadow under and to the
  right of each pane. No borders or separators.
- **Simple** (planned): no gaps and no shadows. Unicode box-drawing borders
  between the orchestrator and the workers, and between each worker.

Each theme has settings that change its colors, so a theme is also a palette:

- The header's foreground and background, and the footer's.
- A worker's title foreground and background.
- The backdrop color and the shadow color (fancy only).

The settings should be per session, so that one session can use one theme and
another a different one, or every session can use the same theme with different
header and footer colors, to tell at a glance which session is which.

In this first version the themes and their settings are constants in
`src/theme.rs`, and the code that draws reads them from there instead of
repeating them. Configuration comes later, once the settings have settled.

## Now

`src/theme.rs` has the fancy theme's colors, `HEADER`, `FOOTER`, `TITLE`,
`BACKDROP`, and `SHADOW`, and its `GAP`. `host::layout` and `TeamChrome` draw
it; `architecture.md` describes what they draw. The orchestrator's pane has no
label: the header names its workspace, and shows "zoomed", "exited N", and
"↑N" at its right end in place of the label's.

## Mockup

`scratch/mockup.py` prints a mockup of the screen with the real glyphs and
escape sequences, so a layout or border style can be tried in seconds, at any
terminal size, without running Hypercube and its agents:

```sh
python3 scratch/mockup.py --cols 100 --rows 30 --workers 2 --focus 1
```

It mirrors the fancy theme. The pane contents are blank, filled with a color so
the panes show, which Hypercube does not do: a pane draws its own cells. A thin
gray border drawn outside the canvas shows where it ends. Its colors are
constants at the top, which `theme.rs` copies. To try a new theme, change the
mockup first, and then change `theme.rs`, `host::layout`, and `TeamChrome` to
match.
