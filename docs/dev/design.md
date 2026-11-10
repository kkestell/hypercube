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
| tab | background and card mixed equally in OKLab | the same mix | Center views, their tab bars, the transcript footer, and source line numbers |
| chart-1 | `0.646 0.222 41.116` | `0.488 0.243 264.376` | Defined; no chart use found |
| chart-2 | `0.6 0.118 184.704` | `0.696 0.17 162.48` | Defined; no chart use found |
| chart-3 | `0.398 0.07 227.392` | `0.769 0.188 70.08` | Defined; no chart use found |
| chart-4 | `0.828 0.189 84.429` | `0.627 0.265 303.9` | Defined; no chart use found |
| chart-5 | `0.769 0.188 70.08` | `0.645 0.246 16.439` | Defined; no chart use found |

Body text uses the foreground token. Components also use transparent, black
and white: black at 50% for dialog overlays, black shadows, white text on
destructive buttons and a white HTML preview background. Gradients fade from transparent to
the surrounding surface. Search highlights use the browser's `Highlight`
and `HighlightText` system colors; search results also use native `mark` styling.

## Status and data colors

Status colors come from [tones](../../internal/ui/web/shared/tones.ts), which
give each meaning one text color. Dots take their tone with `bg-current` or
`border-current`.

| Tone | Color | Uses |
| --- | --- | --- |
| error | destructive token | Errors and failed items; a session's pull request whose checks fail; a branch whose state cannot be read |
| warning | Amber 600 / 500, light / dark | Session waiting on the user; uncommitted branch; modified file; a session's pull request with changes requested; subscription usage at 75% or more |
| info | Sky 600 / 500 | Unseen session reply |
| success | Green 600 / 500 | Running session and transcript dots; unmerged branch; added and untracked files; a session's approved pull request |
| muted | muted-foreground token | Idle and ended sessions; merged or empty branch; deleted file; a session's draft or other pull request; every status icon in the Pull Requests tab |

Running dots breathe by fading their opacity, filled for parent sessions and
outlined for child sessions; with reduced motion they hold still. Other session
dots are filled for parent sessions and outlined for children, except ended
dots, which are always outlined. Subscription usage bars are the stock primary
progress bar.

[Diffs](../../internal/ui/web/changes/diff-view.tsx) shade added lines Green 500
at 15% and removed lines Red 500 at 15%. Added and removed line counts are
muted wherever they show.

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

- Red marks errors, destructive controls, and removed diff lines; the
  destructive token serves both errors and destructive actions.
- Each tone covers several meanings, told apart by context: warning means
  waiting on the user, uncommitted or modified work, and elevated usage;
  success means running, added, unmerged, and approved.
- Theme names do not imply distinct visible colors: secondary, muted and accent
  surfaces have identical values in each theme. Dark terminal colors other than
  its background are delegated to xterm, unlike the explicit light palette.
