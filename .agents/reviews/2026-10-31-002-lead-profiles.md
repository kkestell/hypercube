# Lead profiles and shared Hypercube icon

## Scope and coverage

`git diff f7dd03b..70e0c91`, the implementation of
`agents/plans/2026-10-31-006-lead-profiles.md`. I traced config loading and
validation, lead selection for a new session (CLI, `window new`, view,
`create_agent` refusing lead profiles), `GET /leads`, the `+` menu in
`internal/ui/static/index.html`, and loading of the favicon and the menu bar
icon.

Gaps:

- The `+` menu was reviewed by reading the code; it was not run in a browser.
- The menu bar item was not run. I confirmed instead that macOS decodes
  `favicon.svg` into an `NSImage` and draws it at 18×18 points with visible
  pixels.
- The user's `~/.config/hypercube/config.json` was not read or changed.

## Summary

No findings.

Not fixed, because they are minor or speculative:

- Clicking a `+` button while its menu is open closes the menu and reopens it
  instead of leaving it closed.
- The menu does not move keyboard focus to its first item when it opens.
- A config still in the old format (a top-level `lead` object) fails with
  `invalid role ""`, which does not say that leads moved into `profiles`.

## Checks run

- `make check` with `GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache`: build,
  vet, and race-enabled tests passed, including `TestTmuxNativeLifecycle`.
- A Swift script loaded `favicon.svg` with `NSImage(data:)`, resized it to
  18×18 points, and drew it into a bitmap: it decoded as an SVG image and
  rendered the cube.
- Checked that `ui.css` contains the new menu classes and that every caller of
  `window new` passes a lead.

## Verdict

The change matches the plan and has no high-impact defects in the areas
reviewed.
