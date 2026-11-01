# Choose a lead for each new session, and use the Hypercube icon

Builds on `2026-10-31-005-start-at-login.md` (branch
`hypercube/4a3c98f7/kwork-005`).

## Goal

Every profile in `config.json` has a `role`: `lead`, `plan`, `work`, or
`review`. The single `lead` object goes away; the user can define several lead
profiles. A workspace's `+` button opens a menu that lists the lead profiles,
and the session it creates uses the one the user picks. Leads see each worker
profile's role. The user's config offers Opus and GPT Sol as leads.

The browser view's favicon and the menu bar icon are the Hypercube icon from
the `bun` branch, `src/favicon.svg`.

## Related code

- `internal/harness/config.go` — `Spec`, `Profile`, `Config` (`Lead` and
  `Profiles`), `LoadConfig` validation, `Config.Profile`.
- `cmd/hypercube/main.go` — `openWorkspace(dir, create)` loads the config when
  it adds a team and copies `cfg.Lead` into the new lead; `windowAction`
  handles `window new <state>`; `main` dispatches `window` by argument count
  (4, 6, or 7).
- `internal/mcp/mcp.go` — `toolsFor` builds the `create_agent` profile enum from
  `cfg.Profiles`; `createAgent` resolves `cfg.Profile(args["profile"])`.
- `internal/report/report.go` — the `start` case passes `cfg.Profiles` to
  `facts`; `writeTeamwork` lists the profiles for the lead.
- `internal/ui/ui.go` — `Handler` routes; `window` maps the view's
  `POST /window?action=new&state=...` to `hypercube window new <state>`.
- `internal/ui/static/index.html` — `renderSidebar` draws the workspace row's
  `+` button with `data-action="window?action=new&state=..."`; the `#menu`
  element and the `contextmenu` handler are the existing menu, with its styling
  and its close-on-click, Escape, and blur handling.
- `scripts/dev` — writes the dev config.
- Tests: `internal/harness/harness_test.go`, `internal/mcp/mcp_test.go`,
  `internal/ui/ui_test.go`, `cmd/hypercube/integration_test.go` (config at line
  179, `cli("window", "new", path)` at lines 289 and 334).

## Decisions

- **Schema.** `Profile` gains `Role string \`json:"role"\``, and `Config` keeps
  only `Profiles`. Constants `harness.RoleLead`, `RolePlan`, `RoleWork`, and
  `RoleReview` name the four values. `LoadConfig` rejects any other role and
  requires at least one lead profile and at least one worker profile, which is
  a profile with any other role. Names stay unique across all profiles, and
  every field stays required.
- **Lookups.** `Config.Leads()` and `Config.Workers()` return the profiles of
  each kind in file order. `Config.Lead(name)` returns the named lead profile,
  or the first one when `name` is empty. `Config.Profile` becomes
  `Config.Worker(name)`, which refuses a lead profile, so `create_agent` cannot
  start a worker from a lead profile.
- **What a worker role does.** It is guidance for the lead. `writeTeamwork`
  prints each worker profile as `- name (role; harness, model, effort):
  description`, and its heading says the role is the kind of work the profile
  suits: plan, work, or review. The `create_agent` enum lists worker profile
  names only. Each profile has one role, as the user asked; a profile used for
  two kinds of work gets the role of its main use.
- **Default lead.** Paths that create a session without a choice, which are
  `hypercube start` on a new workspace and the sidebar's new-workspace button,
  use the first lead profile in the file.
- **Choosing a lead.** `openWorkspace(dir string, create bool, lead string)`
  passes `lead` to `cfg.Lead` when it adds a team. `windowAction` takes
  `window new <state> <lead>`, so `main` dispatches `window` with 5, 6, or 7
  arguments. The view always sends the lead's name.
- **Listing leads in the view.** `GET /leads` in `internal/ui/ui.go` loads the
  config and returns `[{"name": ..., "description": ...}]` for the lead
  profiles, or the config error as a 500 with its text. The view fetches it each
  time the menu opens, so config edits show without a restart.
- **The menu.** The workspace row's `+` keeps its icon and its "New Session"
  title and becomes a menu button (`aria-haspopup="menu"`). Clicking it opens
  the existing `#menu` directly below the button, left-aligned with it and kept
  inside the window, with one item per lead profile showing its name and its
  description as the item's tooltip. Choosing an item posts
  `window?action=new&state=...&lead=<name>`. The menu closes the same ways the
  context menu does. A failed fetch shows its error with the view's existing
  `alert`.
- **The user's config.** `docs/configuration.md` and
  `~/.config/hypercube/config.json` must stay the same. The worker changes the
  example in `docs/configuration.md` only. The lead copies it to the user's
  file when the user installs this change, because the installed build rejects
  the new format.

- **The icon.** `git show bun:src/favicon.svg` becomes
  `internal/ui/static/favicon.svg`, the one copy of the icon. `index.html`
  links it with `<link rel="icon" href="favicon.svg" type="image/svg+xml">`;
  its own style block turns it white in a dark browser theme. `ui.Icon()
  []byte` returns the file from the embedded `static` files, and `serveUI`
  passes it to `menu.Run(url, icon)`. The Objective-C side loads it with
  `[[NSImage alloc] initWithData:]`, which reads SVG on current macOS, sets
  its size to 18×18 points, and marks it a template, replacing the SF Symbol
  `cube`. A template image uses only the alpha channel, so the menu bar draws
  it in its own color in light and dark mode.

The new example, applied to the current one:

- Replace the `lead` object with two profiles at the top of `profiles`:
  `opus` (`claude`, `opus`, `high`, description "Opus at high effort in Claude
  Code.") and `sol` (`codex`, `gpt-6.1-sol`, `high`, description "GPT Sol at
  high effort in Codex."), both with role `lead`.
- Roles for the existing profiles: `plan` for `claude-large`,
  `claude-opus-high`, and `claude-fable`; `review` for `gpt-medium` and
  `claude-medium`; `work` for every other profile.

## Test plan

- `TestLoadConfig` (`internal/harness/harness_test.go`): a config with one lead
  and one worker loads; `Lead("")` returns the first lead, `Lead("name")` the
  named one, and `Lead("worker-name")` fails; `Worker` finds the worker and
  refuses the lead's name. Rejected: a missing or unknown role, no lead profile,
  no worker profile, a duplicate name across roles, and the old top-level
  `lead` object without lead profiles.
- `internal/mcp/mcp_test.go`: the `create_agent` enum lists worker names only;
  `create_agent` with a lead profile's name fails.
- `internal/report` tests: the lead's start context lists worker profiles with
  their roles and leaves lead profiles out.
- `internal/ui/ui_test.go`: `GET /leads` returns the lead profiles in file
  order; a broken config returns 500 with the error.
- `TestTmuxNativeLifecycle`: the config has two lead profiles with different
  models. `hypercube start` creates the first session with the first lead's
  model; `cli("window", "new", path, "second")` creates a session whose lead has
  the second lead's model; an unknown lead name fails and creates nothing.
- Live, in `make dev`: the `+` menu lists the leads, opens below the button,
  closes on Escape and on outside clicks, and each item starts a session with
  that lead's harness.
- `internal/ui/ui_test.go`: `GET /favicon.svg` returns the icon, and `Icon()`
  returns the same bytes.
- Live, run by the user after installing: the browser tab shows the Hypercube
  icon in light and dark themes, and the menu bar shows it legibly in light and
  dark menu bars.

## Implementation plan

1. `internal/harness/config.go`: add `Role` and the role constants; remove
   `Config.Lead`; validate roles and counts; add `Leads`, `Workers`, and `Lead`;
   rename `Profile` to `Worker` and make it refuse lead profiles.
2. `cmd/hypercube/main.go`: add the `lead` parameter to `openWorkspace` and use
   `cfg.Lead(lead)`; pass `""` from `start` and `restoreWorkspaces`; take
   `window new <state> <lead>` in `windowAction` and adjust the dispatch counts.
3. `internal/mcp/mcp.go`: build the enum from `cfg.Workers()`; use
   `cfg.Worker` in `createAgent`.
4. `internal/report/report.go`: pass `cfg.Workers()` to `facts`; print roles in
   `writeTeamwork` and name them in its heading.
5. `internal/ui/ui.go`: add `GET /leads`; pass `lead` in `window`'s `new` case.
6. `internal/ui/static/index.html`: turn the workspace row's `+` into the menu
   button and fill `#menu` from `leads`.
7. `scripts/dev`: write lead profiles with roles in the dev config.
8. `docs/configuration.md`: the new example and text.
9. `internal/ui/static/favicon.svg` (new, from `bun:src/favicon.svg`); link it
   in `index.html`; add `ui.Icon()`.
10. `internal/menu/menu.go` and `internal/menu/menu.m`: take the icon bytes in
    `Run` and load them as the status item's template image; `serveUI` passes
    `ui.Icon()`.
11. Tests from the test plan.

## Documentation updates

- `docs/configuration.md`: the example as above. The text says every profile
  has a role; `lead` profiles are the leads offered by a workspace's `+` menu,
  and the first one is used when a session starts without a choice; `plan`,
  `work`, and `review` profiles are offered to leads for workers, and the role
  tells the lead what work the profile suits. At least one lead and one worker
  profile are required.
- `docs/usage.md`: hovering over a workspace's row and clicking `+` opens a
  list of lead profiles; choose one to create a session with it.
