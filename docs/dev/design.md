# Interface colors

This is an inventory of the desktop app's browser interface, based on the
source, not a proposed color system. Token names suggest roles; the examples
below record where the code actually uses them. Colors in terminal output,
file content, previews, native window controls, and third-party themes may be
chosen outside Hypercube. Values and uses can change with dependencies.

## Theme colors

[base.css](../../internal/ui/web/base.css) defines these light and dark values in
OKLCH. Opacity modifiers on utilities produce additional translucent colors.

| Token | Light | Dark | Observed use |
| --- | --- | --- | --- |
| background | `1 0 0` | `0.145 0 0` | Main surface, dialogs, find controls |
| foreground | `0.145 0 0` | `0.985 0 0` | Explicit primary text, active tabs, usage marker |
| card | `1 0 0` | `0.205 0 0` | Cards and alerts |
| card-foreground | `0.145 0 0` | `0.985 0 0` | Card text |
| popover | `1 0 0` | `0.205 0 0` | Menus, selects, popovers |
| popover-foreground | `0.145 0 0` | `0.985 0 0` | Their text |
| primary | `0.205 0 0` | `0.922 0 0` | Primary buttons, checked controls, progress and usage ring |
| primary-foreground | `0.985 0 0` | `0.205 0 0` | Text or marks on primary surfaces |
| secondary | `0.97 0 0` | `0.269 0 0` | Secondary buttons, selected session hover, user message bubbles |
| secondary-foreground | `0.205 0 0` | `0.985 0 0` | Secondary button text |
| muted | `0.97 0 0` | `0.269 0 0` | Subdued surfaces, keyboard labels, code blocks, tabs |
| muted-foreground | `0.556 0 0` | `0.708 0 0` | Secondary text, labels, placeholders, inactive icons; hovered scrollbar at 50% |
| accent | `0.97 0 0` | `0.269 0 0` | Hover, focus and selection surfaces; selected source lines |
| accent-foreground | `0.205 0 0` | `0.985 0 0` | Text on interactive accent surfaces |
| destructive | `0.577 0.245 27.325` | `0.704 0.191 22.216` | Errors, failed items, invalid fields and destructive component variants |
| border | `0.922 0 0` | `1 0 0 / 10%` | Default borders and separators |
| input | `0.922 0 0` | `1 0 0 / 15%` | Control borders and some control backgrounds |
| ring | `0.708 0 0` | `0.556 0 0` | Focus outlines and rings |
| sidebar | `0.985 0 0` | `0.205 0 0` | Sidebar and right panel surfaces |
| chart-1 | `0.646 0.222 41.116` | `0.488 0.243 264.376` | Defined; no chart use found |
| chart-2 | `0.6 0.118 184.704` | `0.696 0.17 162.48` | Defined; no chart use found |
| chart-3 | `0.398 0.07 227.392` | `0.769 0.188 70.08` | Defined; no chart use found |
| chart-4 | `0.828 0.189 84.429` | `0.627 0.265 303.9` | Defined; no chart use found |
| chart-5 | `0.769 0.188 70.08` | `0.645 0.246 16.439` | Defined; no chart use found |

The body instead defaults to Tailwind `neutral-700` in light mode and
`neutral-300` in dark mode. Components also use transparent, black and white:
black at 50% for dialog overlays, black shadows, white text on destructive
buttons and a white HTML preview background. Gradients fade from transparent to
the surrounding surface. Search highlights use the browser's `Highlight`
and `HighlightText` system colors; search results also use native `mark` styling.

## Status and data colors

Tailwind names below identify the actual palette utilities, including their
opacity where specified. See [session status](../../internal/ui/web/sidebar/session-item.tsx),
[status dots](../../internal/ui/web/shared/status-dot.tsx),
[branch icons](../../internal/ui/web/shared/branch-icon.tsx),
[files](../../internal/ui/web/files/files-panel.tsx),
[diffs](../../internal/ui/web/changes/diff-view.tsx), and
[limits](../../internal/ui/web/agent/limits.tsx).

| Color | Observed use |
| --- | --- |
| Amber 500 | Session waiting on the user; subscription usage at 75–89%; dark uncommitted branch icon |
| Amber 600 / 500 | Uncommitted branch icon, light / dark |
| Amber 700 / 500 at 70% | Modified file name, light / dark |
| Sky 500 | Unseen session reply |
| Neutral 600, neutral 500 outline | Idle session, filled / outlined respectively; ended sessions always use the neutral 500 outline |
| Neutral 500, 20% track | Subscription usage below 75% and its track |
| Green 600 / 500 | Unmerged branch icon, light / dark |
| Green 700 / 500 at 70% | Added and untracked file names, light / dark |
| Green 500 at 15% | Added diff line background |
| Red 500 at 15% | Removed diff line background |
| Red 500 | Subscription usage at 90% or more |
| Green 600 to 400 | Running session and transcript dots breathe between these shades |

Running dots breathe between `green-600` (`#16a34a`) and `green-400`
(`#4ade80`), filled for parent sessions and outlined for child sessions.
Other session dots are filled for parent sessions and outlined for children,
except ended dots, which are always outlined. Transcript running dots also use
the shared busy-dot component.

## Other palettes

- The Claude harness icon uses `#D97757`; the other harness icons inherit
  their text color. The menubar SVG uses black as a template; the favicon is
  black by default and white under the dark color-scheme media query.
- [Terminal](../../internal/ui/web/terminal/xterm.ts) light mode sets background
  `#ffffff`, foreground and cursor `#252525`, cursor accent `#ffffff`, and
  selection `#b5d5ff`. Its ANSI palette is black `#000000`, red `#cd3131`,
  green `#00a000`, yellow `#795e00`, blue `#0451a5`, magenta `#bc05bc`, cyan
  `#0598bc`, white `#555555`; bright black `#666666`, bright red `#cd3131`,
  bright green `#008000`, bright yellow `#795e00`, bright blue `#0451a5`,
  bright magenta `#bc05bc`, bright cyan `#0598bc`, and bright white `#000000`.
  Dark mode specifies only background `#0a0a0a` and leaves the rest to xterm's
  defaults. ANSI colors represent terminal output selected by programs, not
  fixed Hypercube status meanings.
- [Syntax highlighting](../../internal/ui/web/files/highlight.ts) uses Shiki's
  GitHub light and dark default themes. Token colors represent language syntax
  according to those external themes; the palettes are supplied by Shiki rather
  than defined here. Rendered HTML and images can contain arbitrary colors.
- Native desktop chrome follows the selected system, light or dark appearance;
  its colors are supplied by the operating system.

## Inconsistencies to revisit

- The development convention reserves red for errors, but removed diff lines
  and high subscription usage also use red. Destructive controls use the red
  destructive token, which also marks errors.
- Amber means waiting, modified or uncommitted work, and elevated usage; green
  means added files, added diff lines and unmerged branches, as well as the
  working-session indicator. These meanings depend on context.
- The same Git change is colored differently by surface: modified file names
  and uncommitted branch icons use different amber shades; added file names and
  diff backgrounds use different greens. Deleted files are muted while removed
  diff lines are red. Added and untracked files share a color.
- Body text uses neutral utilities rather than the foreground token.
- Theme names do not imply distinct visible colors: secondary, muted and accent
  surfaces have identical values in each theme. Dark terminal colors other than
  its background are delegated to xterm, unlike the explicit light palette.
