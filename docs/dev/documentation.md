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
| How-to | Complete a particular task. | `installation.md`, `usage.md`, `teamwork.md`, `review-changes.md`, `upgrades.md`, `troubleshooting.md` |
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
`docs/screenshots/`.

In guides, show each screenshot with `<picture>`: a `<source
media="(prefers-color-scheme: dark)">` pointing to `images/<scene>-dark.png`
and an `<img>` pointing to `images/<scene>-light.png`. Do not regenerate
screenshots just for prose changes.

The [capture driver](../../scripts/screenshots/main.js) runs a real backend with
the [scripted plugin](../../plugins/scripted/main.go), opens the desktop app,
and captures that window, including its native buttons and rounded corners,
then adds a transparent margin with a drop shadow.
Temporary resources are deleted afterward.

The driver puts a stand-in `gh` first on the backend's `PATH`. In a scene with
`github: true` it is signed in, and the backend reads GitHub from the driver's
own GraphQL server through `HYPERCUBE_GITHUB_API`; in other scenes it has no
accounts. `docs/screenshots/repos/github.json` names the account and
gives each fixture repository its `github.com` name, how many commits its
`main` is `behind`, and its open `pullRequests`. Each workspace's `origin` is
that `github.com` address; Git fetches it from a local bare repository through
`url.insteadOf`.

A pull request declares its `number`, `title`, and head `branch`, and
optionally its `author` (the account by default), `draft`, `review` (GitHub's
review decision), `checks`, `merge` (GitHub's merge state, `BLOCKED` by
default), `pendingReviews`, `requested` (`user` or `team`), the account's own
`reviewed` state, `unanswered` review threads, and `requiredFailed`.

### Scene contents

Each scene declares:

- Workspaces copied from fixture repositories in `docs/screenshots/repos/`,
  with optional patches of uncommitted changes.
- Session settings as shown in the app: harness, model, and effort.
- Turn prompts, transcript items in plugin-protocol JSON, and ending state
  (`idle`, `working`, or `ask`). A turn's `usage` and `limits` arrive with its
  end; each limit gives a window's `minutes`, percent `used`, and minutes
  `left` until it resets.
- Children, task offers, and agent-initiated suggestions.
- `openModelPicker: true` opens the composer's model picker before capture.
  `openSettings` opens Settings at the named section, and `openSearch` opens
  session search with the given query.
- `terminal` opens the selected session's terminal panel and runs the given
  command there. The shell is zsh with a prompt naming only its directory.
- `waiting: true` captures the waiting page offering **Set Up Background
  Service** instead of a backend's page; such a scene declares only `size`.
- Optional fixture-file writes and Git steps; `worktree` gives a top-level
  session its own branch.
- `parked: true` parks a top-level session and its children: they play first,
  then the backend quits and restarts before the other sessions play.
  `unseen: true` leaves a session's reply unread, which shows a notification.
- Saved tabs and splits in `view`; `view.expanded` names `activity` or item IDs
  to expand in a saved turn. `select` chooses the session as a workspace and
  session index, a workspace's dashboard as a workspace index alone, or, empty,
  the Pull Requests view; `size` sets window dimensions in points.
- A workspace's `pullRequests` adds open pull requests to its repository's.
  `session` names a worktree session by its first prompt, and the pull request
  comes from that session's branch.
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
