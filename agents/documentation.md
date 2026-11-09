# Documentation maintenance

User documentation starts in [README.md](../README.md) and lives in `docs/`.
Contributor setup starts in [CONTRIBUTING.md](../CONTRIBUTING.md); architecture
and project rules live in `agents/`. Keep these audiences separate.

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

## Screenshots

Run `make screenshots` only when captures need changing. It regenerates
`docs/images/<scene>-light.png` and `-dark.png` from scenes in
`docs/screenshots/`.

The [capture driver](../scripts/screenshots/main.js) runs a real backend with
the [scripted plugin](../plugins/scripted/main.go), opens the desktop app,
and captures that window, including its native buttons and rounded corners.
Temporary resources are deleted afterward.

### Scene contents

Each scene declares:

- Workspaces copied from fixture repositories in `docs/screenshots/repos/`,
  with optional patches of uncommitted changes.
- Session settings as shown in the app: harness, model, and effort.
- Turn prompts, transcript items in plugin-protocol JSON, and ending state
  (`idle`, `working`, or `ask`).
- Children, task offers, and agent-initiated suggestions.
- Optional fixture-file writes and Git steps; `worktree` gives a top-level
  session its own branch.
- Saved tabs and splits in `view`; `view.expanded` names `activity` or item IDs
  to expand in a saved turn. `select` chooses the session; `size` sets window
  dimensions in points.
- `workbench.rightTab` chooses Files or Git (`files` or `git`);
  `comparison: "branch"` selects Git's Branch changes view for a worktree.

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
transcript filling its column, and the Files panel open. Commit new captures
with their scene changes.

## Website

- `make web-build` builds into `web/public/`.
- `make web-serve` previews at http://localhost:1313/.
- Both use the Hugo version pinned in `web/Makefile`; `make check` includes
  the website build.

Hugo publishes `docs/*.md` at `/docs/` and `docs/images/` at `/docs/images/`.
Screenshot scenes and fixtures are not published. Guides use relative `.md`
links to one another; links outside the published guides use GitHub URLs.
Keep shared Markdown H1s and YAML frontmatter (`title`, `weight`); the website
suppresses duplicate H1s.

In shared guides, use Markdown images pointing to `images/<scene>-light.png`.
The website's image renderer resolves the path and supplies the dark variant;
GitHub displays the light image. Raw `<picture>` markup is omitted by Hugo.
The README is not published through Hugo and can use `<picture>` for both
themes. Do not regenerate screenshots just for prose changes.

For Cloudflare Pages, use the repository root, build command `make web-build`,
and output directory `web/public`. That build does not capture screenshots.
Keep generated output gitignored. See [website instructions](../web/AGENTS.md)
before changing site configuration or layouts.
