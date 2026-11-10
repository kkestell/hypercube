# Documentation maintenance

User documentation starts in [README.md](../../README.md) and lives in `docs/*.md`.
Contributor setup starts in [CONTRIBUTING.md](../../CONTRIBUTING.md); architecture
and project rules live in `docs/dev/`. Research, plans, reviews, scratch work,
and work logs live in `.agents/`. Keep these audiences separate.

## Structure and writing

Use [Diátaxis](https://diataxis.fr/) to choose a page's purpose:

| Type | Reader's need | Existing pages |
| --- | --- | --- |
| Tutorial | Learn by completing a safe, concrete exercise. | `getting-started.md` |
| How-to | Complete a particular task. | `installation.md`, `usage.md`, `teamwork.md`, `workflows.md`, `review-changes.md`, `upgrades.md`, `troubleshooting.md` |
| Reference | Look up precise behavior, values, or limits. | `configuration.md`, `harness-support.md`, `interface.md`, `storage.md` |
| Explanation | Understand relationships and tradeoffs. | `introduction.md`, `lifecycle.md` |

- Give each page one purpose. Link to another type rather than interrupting a
  procedure with a reference dump or an architectural explanation.
- Describe supported product workflows, not every entry point the
  implementation exposes. User guides cover the desktop app; its web server
  and development browser view are not user-facing alternatives.
- Start with the outcome and prerequisites. Use task headings, short
  paragraphs, numbered procedures, and tables for lookup.
- Use screenshots where readers need to locate a control or recognize a
  result. Place them beside the relevant step and explain what to look for.
- Keep required steps separate from optional choices. Put destructive effects
  before the command, and distinguish macOS from Linux.
- Document public defaults, not a maintainer's personal settings or model
  rankings. Do not change local configuration when editing documentation.
- Keep each fact in one authoritative location. Verify behavior against code;
  correct or remove inaccuracies rather than accumulating caveats.
- Add a guide only when requested or when a requested documentation
  restructuring needs it. Update the README links when adding or moving a guide.
- Preserve existing page paths when practical; update live links and anchors
  when moving content. Historical plans and reviews are not current guides.

For prose-only changes, inspect the diff and links; do not run code checks.

## Architecture diagram

The diagram in `docs/dev/architecture.md` uses `docs/dev/images/architecture.png`,
rendered from the editable `docs/dev/images/architecture.svg`. Update the SVG
against the current executable names, process ownership, and transports, then
regenerate the PNG from the repository root with librsvg:

```sh
rsvg-convert --zoom 2 -o docs/dev/images/architecture.png docs/dev/images/architecture.svg
```

Open the PNG to check labels and arrows. Commit both source and rendered image.

## Screenshots

Run `make screenshots` only when captures need changing. It regenerates
`docs/images/<scene>-light.png` and `-dark.png` from scenes in
`docs/screenshots/` in seconds, with no backend, display, or permissions.

In guides, show each screenshot with `<picture>`: a `<source
media="(prefers-color-scheme: dark)">` pointing to `images/<scene>-dark.png`
and an `<img>` pointing to `images/<scene>-light.png`. Do not regenerate
screenshots just for prose changes.

The [capture driver](../../scripts/screenshots/main.js) serves the built page
from a server in its own process that answers from the scene. It renders the
desktop window off screen at twice the scene's size, then draws the macOS
window frame: rounded corners, a hairline edge, the window buttons, and a drop
shadow on a transparent margin. A request the scene does not answer, or a step
that does not finish within five seconds, fails the run.

### Scene contents

A scene holds what the backend sends the page, and what the driver does in the
page before capturing it:

- `size`: the window's dimensions in points.
- `now`: the page's clock, in milliseconds since the epoch, so relative times
  stay as written.
- `address`: the page's hash: a session ID, `workspace:<ID>` for a workspace's
  dashboard, or empty for the Pull Requests view.
- `views`: the saved workbench, tabs, splits, and expanded items, as the page
  keeps them in local storage under `hypercube.views`. Editor panes give the
  conversation two fifths of the width beside the sidebar and right panel.
- `sockets`: each socket's messages by path and query. `/state` sends `state`
  and `transcript` messages as `json`; a terminal socket sends shell output as
  base64 `binary`.
- `responses`: HTTP responses by method, path, query, and any request body,
  each with `status` and `json`, or `type` and `body` or `base64`. POST
  requests the scene does not list succeed with no body. The driver sets the
  config response's theme.
- `actions`: `openModelPicker` opens the composer's model picker;
  `openSettings` opens Settings at the named section; `openSearch` opens
  session search with the given query; `newSession` opens a new session in the
  workspace with that ID, and `newBranch` then opens **Create a branch** and
  types the branch name; `sessionMenu` chooses `item` from the menu of
  `session`; `comparison: "branch"` selects Git's Committed view; `terminal`
  waits for the terminal to show the given command.
- `waiting: true` with only `size` captures the waiting page offering **Set Up
  Background Service**.

Message and response fields follow the types in `internal/ui/web`, and paths
are under `/Users/demo`. Start a new scene from the closest existing one.

### Checking captures

Open every generated image and check theme, content, toolbar layout, and
window buttons. Use the app's default panel sizes. Keep toolbars fully visible
and session labels readable. Use several sessions across workspaces, a
transcript filling its column, and the right panel open unless a scene has file
tabs and does not need the file tree or Git changes; the sidebar, conversation,
file tabs, and right panel together leave too little room. Commit new captures
with their scene changes.
