# Application configuration package

## Goal

`config.json` holds the user's preferences: default selection, child-model
guidance, default worktree, theme, terminal font size, and notifications. Its
code lives in `internal/harness/config.go`, so the package that defines the
plugin protocol also owns the app's preferences. This task moves that code to
a new package, `internal/config`, and points every caller at it.

When the work is done, `internal/config` owns the document, its defaults,
validation, atomic writes, and upgrades of earlier versions' files, and
`internal/harness` holds no configuration code. The file's location, format,
defaults, and upgrade behavior are unchanged.

This is the first of six plans from the
[runtime boundaries review](../reviews/2026-11-07-007-runtime-boundaries.md)
(finding F4, configuration part). It has no prerequisite.

## Related code

- `internal/harness/config.go` — `Config`, `Notifications`,
  `DefaultNotifications`, `themes`, `terminalFontSizes`, `configPath`,
  `LoadConfig`, `Config.Validate`, `SaveConfig`, `writeConfig`,
  `legacyProfile`, the `upgrade` type, `UpgradeConfig`, `upgradeProfiles`,
  and `upgradeNotifications`.
- `internal/harness/config_test.go` — defaults, missing and malformed files,
  validation, complete writes, profile and notification upgrades, and leaving
  an unchanged file alone. It constructs `Settings` unqualified.
- `internal/harness/discovery.go` — `Settings` and `Settings.Check`, which
  `Config.DefaultSelection` and `legacyProfile` use. `Settings` stays in
  `harness`; it is part of the plugin protocol's discovery data.
- Callers:
  - `cmd/hypercube/main.go:85` — `harness.UpgradeConfig()` in `startApp`.
  - `cmd/hypercube/main.go:127-128` — `harness.DefaultNotifications` and
    `harness.LoadConfig()` in `sendAlert`.
  - `cmd/hypercube/integration_test.go:287` — decodes `/config` into
    `harness.Config`.
  - `internal/session/launch.go:217` — `harness.LoadConfig()` in `startArgs`
    for `ChildGuidance`.
  - `internal/session/selection_test.go:192` — `harness.SaveConfig`.
  - `internal/ui/config.go:12,23,38` — `GET /config` and `POST /config`.
  - `internal/ui/config_test.go:21-42`, `internal/ui/ui_test.go:88` — build
    and read `harness.Config` values.

## Decisions

### Names

The package name says what the functions act on, so they drop the `Config`
suffix:

| Old (`harness`) | New (`config`) |
| --- | --- |
| `Config` | `Config` |
| `Notifications` | `Notifications` |
| `DefaultNotifications` | `DefaultNotifications` |
| `LoadConfig()` | `Load()` |
| `SaveConfig(c)` | `Save(c)` |
| `UpgradeConfig()` | `Upgrade()` |
| `(*Config).Validate` | `(*Config).Validate` |
| `configPath` | `path` |
| `writeConfig` | `write` |

`legacyProfile`, `upgrade`, `upgradeProfiles`, `upgradeNotifications`,
`themes`, and `terminalFontSizes` keep their names. The old declarations are
deleted; `harness` keeps no aliases.

`Load` and `Upgrade` both build the default document inline:
`Config{Theme: "system", TerminalFontSize: 13, Notifications:
DefaultNotifications}`. The move replaces both with one private function:

```go
// defaults is the configuration with every preference at its default. A file
// written before a preference existed takes that default.
func defaults() Config {
	return Config{Theme: "system", TerminalFontSize: 13, Notifications: DefaultNotifications}
}
```

### The package

`internal/config/config.go`:

```go
// Package config reads and writes config.json, the user's preferences, in
// $XDG_CONFIG_HOME/hypercube or ~/.config/hypercube. Upgrade converts files
// written by earlier versions before anything reads them.
package config

import "github.com/kkestell/hypercube/internal/harness"

type Config struct {
	DefaultSelection *harness.Settings `json:"default_selection,omitempty"`
	ChildGuidance    string            `json:"child_model_guidance"`
	DefaultWorktree  bool              `json:"default_worktree"`
	Theme            string            `json:"theme"`
	TerminalFontSize int               `json:"terminal_font_size"`
	Notifications    Notifications     `json:"notifications"`
}

type legacyProfile struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	harness.Settings
}
```

The doc comments, JSON names, error wrapping (`"%s: %w"` with the path,
`"saving config: %w"`, and so on), and upgrade order (`upgradeProfiles`, then
`upgradeNotifications`) carry over unchanged.

`config` imports `harness` for `Settings`. `harness` still imports `hold`
until the next plan moves the plugin client out, so `config` reaches `hold`
transitively until then.

## Test plan

- `internal/harness/config_test.go` moves to `internal/config/config_test.go`
  as `package config`, with `Settings` written `harness.Settings` and the
  renamed functions. Every existing case keeps its assertions: default
  values without a file, malformed JSON, invalid theme and font size, invalid
  default selection, `Save` writing the complete document, profile upgrades
  (default profile, first profile when none is named, existing guidance
  kept), `macos` renamed to `desktop`, and an unchanged file left byte for
  byte.
- `internal/ui` `TestConfig` and the config tests in `config_test.go` pass
  with `config.Config`.
- `internal/session` selection tests pass with `config.Save`.
- `cmd/hypercube` `TestAppProcess` still reads an upgraded document from
  `/config` (`DefaultSelection` set, guidance containing `model p/m, effort
  high: test`).
- `go list -deps ./internal/harness` no longer lists anything for config;
  `grep -rn 'harness\.\(Config\|LoadConfig\|SaveConfig\|UpgradeConfig\|Notifications\|DefaultNotifications\)' --include='*.go' .`
  finds nothing.

## Implementation plan

1. Create `internal/config/config.go` from `internal/harness/config.go` with
   the package comment, renames, `defaults()`, and `harness.Settings`
   qualifications above. Delete `internal/harness/config.go`.
2. Move `internal/harness/config_test.go` to `internal/config/config_test.go`
   and update it as described in the test plan.
3. Update the callers listed in Related code to import
   `github.com/kkestell/hypercube/internal/config` and use `config.Load`,
   `config.Save`, `config.Upgrade`, `config.Config`, `config.Notifications`,
   and `config.DefaultNotifications`. Remove `harness` imports that become
   unused (for example in `internal/ui/config.go`). In
   `internal/session/launch.go` the local variable `cfg` stays; the call
   becomes `config.Load()`.
4. Run `go build ./...` and the grep above to catch callers outside the list.
5. Update the documentation and run `make check`.

## Documentation updates

- `agents/development.md`, Code: add `config` to the `internal/` package list.
