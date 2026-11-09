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
| How-to | Complete a particular task. | `installation.md`, `usage.md`, `teamwork.md`, `review-changes.md`, `upgrades.md` |
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
  restructuring needs it. Update the README and website landing links together.
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
`docs/screenshots/`.

The [capture driver](../../scripts/screenshots/main.js) runs a real backend with
the [scripted plugin](../../plugins/scripted/main.go), opens the desktop app,
and captures that window, including its native buttons and rounded corners,
then adds a transparent margin with a drop shadow.
Temporary resources are deleted afterward.

### Scene contents

Each scene declares:

- Workspaces copied from fixture repositories in `docs/screenshots/repos/`,
  with optional patches of uncommitted changes.
- Session settings as shown in the app: harness, model, and effort.
- Turn prompts, transcript items in plugin-protocol JSON, and ending state
  (`idle`, `working`, or `ask`).
- Children, task offers, and agent-initiated suggestions.
- `openModelPicker: true` opens the composer's model picker before capture.
- Optional fixture-file writes and Git steps; `worktree` gives a top-level
  session its own branch.
- `parked: true` parks a top-level session and its children: they play first,
  then the backend quits and restarts before the other sessions play.
  `unseen: true` leaves a session's reply unread, which shows a notification.
- Saved tabs and splits in `view`; `view.expanded` names `activity` or item IDs
  to expand in a saved turn. `select` chooses the session; `size` sets window
  dimensions in points.
- `workbench.rightPanel: false` hides the right panel; `workbench.rightTab`
  chooses Files or Git (`files` or `git`); `comparison: "branch"` selects Git's
  Committed view for a worktree. The driver sizes the editor panes so the
  conversation takes two fifths of the width beside the sidebar and right panel.

### Capture requirements

Use a Retina display large enough for the scene's window. Native window capture
requires macOS Screen Recording permission for the app running the command.
If permission is denied, stop and wait for the user to resolve it; do not change
permissions automatically. Run one capture at a time and keep the screenshot
window in front: a covered window can stop painting, and the driver rejects a
capture that differs from the rendered page.

Open every generated image and check theme, content, toolbar layout, and native
window buttons.
Use the app's default panel sizes. Keep toolbars fully visible and session
labels readable. Use several sessions across workspaces, a
transcript filling its column, and the right panel open unless a scene has file tabs
and does not need the file tree or Git changes; the sidebar, conversation, file
tabs, and right panel together leave too little room. Commit new captures
with their scene changes.

## Website

- `make web-build` builds into `web/public/`.
- `make web-serve` previews at http://localhost:1313/.
- Both use the Hugo version pinned in `web/Makefile`; `make check` includes
  the website build.

Hugo publishes `docs/*.md` at `/docs/` and `docs/images/` at `/docs/images/`.
Developer references in `docs/dev/`, screenshot scenes, and fixtures are not
published. Guides use relative `.md`
links to one another; links outside the published guides use GitHub URLs.
Guides have no frontmatter. Keep each Markdown H1; the website suppresses
duplicate H1s. Page titles and menu order live in `[[cascade]]` entries in
`web/hugo.toml`; add an entry when adding a guide.

In guides, show each screenshot with `<picture>`: a `<source
media="(prefers-color-scheme: dark)">` pointing to `images/<scene>-dark.png`
and an `<img>` pointing to `images/<scene>-light.png`. The site publishes pages
as `/docs/<name>.html`, so the same relative paths work on GitHub and the
website. Do not regenerate screenshots just for prose changes.

For Cloudflare Pages, use the repository root, build command `make web-build`,
and output directory `web/public`. That build does not capture screenshots.
Keep generated output gitignored. See [website instructions](../../web/AGENTS.md)
before changing site configuration or layouts.
