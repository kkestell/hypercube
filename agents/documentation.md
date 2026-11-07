# Documentation

The example in `docs/configuration.md` is the user's
`~/.config/hypercube/config.json`. Change both together so they stay the same.

`README.md` indexes the user documentation in `docs/`. Update a guide only
when the behavior it describes changes. `AGENTS.md` is the entry point for
agent instructions and should rarely change. Developer reference lives in
[architecture](architecture.md) and
[development and releases](development.md). Outstanding work belongs in
[GitHub Issues](https://github.com/kkestell/hypercube/issues).

The code describes the implementation. A fact lives in one place. Correct or
delete inaccurate text without expanding it. Do not create new user guides
unless asked.

## Documentation screenshots

```sh
make screenshots
```

This regenerates `docs/images/<scene>-light.png` and `<scene>-dark.png` for
each scene in `docs/screenshots/`. A scene file lists workspaces, each a copy
of a fixture repository in `docs/screenshots/repos/` with an optional patch of
uncommitted changes, and their sessions: each session's `settings` (harness,
model, and effort, as the session shows them), each turn's prompt, the transcript
items in the plugin protocol's JSON, how the turn ends (`idle`, `working`, or
`ask`), the children it starts, the tasks it offers, and its own-initiative
suggestions. A turn may write fixture files and run Git commands in its session
directory with `git` steps; `worktree` starts a top-level session on its own
branch. A session's `view` is the page's saved tabs and splits; `view.expanded`
names `activity` or item IDs to open in a saved turn. `select` picks the shown
session, and `size` is the window in points.
[scripts/screenshots/main.js](../scripts/screenshots/main.js) plays each scene
through a real backend with [the scripted plugin](../plugins/scripted/main.go),
opens the real desktop window, and captures it. Everything else it creates is
deleted.

The window must fit on a Retina display. On first use macOS asks to grant the
terminal the Screen Recording permission. Run one capture at a time in one
visible terminal window, and leave the screenshot window in front until the run
ends: a covered window stops painting, and the driver fails a capture that
differs from the page.

Open every image the run writes and check its theme, its content, and the
window's title bar buttons. Give a new scene the density of the existing ones:
several sessions across workspaces, a transcript that fills its column, and the
Files panel open.

## Website

`make web-build` builds the Hugo site into `web/public/`; `make web-serve`
previews it at http://localhost:1313/. Both use the pinned Hugo version in
`web/Makefile`. `make check` includes the website build.

The guides in `docs/` and screenshots in `docs/images/` are shared by GitHub
and the website. Hugo mounts them at `/docs/` and `/docs/images/`; screenshot
scenes and fixtures in `docs/screenshots/` are not published. In the guides,
link to other guides with relative `.md` paths and screenshots with
`images/<scene>-light.png`. Hugo resolves the guide URLs and adds each
screenshot's `-dark.png` variant for dark appearance.

For Cloudflare Pages, use the repository root as the build root,
`make web-build` as the build command, and `web/public` as the output directory.
Commit screenshots generated locally with `make screenshots`; the website
build does not run the macOS screenshot capture. Keep `web/public/` gitignored.
