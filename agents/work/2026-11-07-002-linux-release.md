# Linux release

## Plan

`agents/plans/2026-11-07-002-linux-release.md`

## Summary

`release.yml` now runs a `macos` job, a `linux` matrix job
(`ubuntu-24.04`, `ubuntu-24.04-arm`), and a `release` job that runs only on a
tag push and creates one draft prerelease with the ZIP, both tarballs, and one
`SHA256SUMS`. `workflow_dispatch` packages under `v0.0.0-beta.1` and keeps the
archives as artifacts. `scripts/linux/test-release` runs last in
`make linux-test`. `AGENTS.md`, `README.md`, `docs/installation.md`, and
`agents/development.md` describe Linux. The workflow itself is unrun, so the
goal is met up to what only a push can show.

## Departures from the plan

- `scripts/release` chose `shasum` for `SHA256SUMS`, which Fedora's image lacks.
  It now uses `sha256sum` when present and `shasum -a 256` otherwise. The plan
  assumed the script already worked on every distribution.
- `actionlint` did not run: the auto-mode classifier denied
  `go run github.com/rhysd/actionlint/cmd/actionlint@latest` as external code.
  I parsed the YAML with Ruby and checked the job graph instead.

## Decisions

- Artifact actions use `upload-artifact@v7` (the version `check.yml` uses) and
  `download-artifact@v8` (the latest release).
- Artifacts are named `macos` and `linux-<runner>`; the release job merges them
  into `archives/` and runs `gh release create` from there.
- `test-release` copies the checkout with `tar --exclude=./bin
  --exclude=node_modules`, so it needs no list of ignored files.
- The README keeps "fullscreen" in the native desktop controls bullet; the plan
  omitted it, but the text still holds.

## Checks run

- `scripts/linux/test-release` on Debian, Ubuntu, and Fedora (ARM64) — passed.
- `make linux-test` on Debian, Ubuntu, and Fedora — all four tests passed on
  each.
- `make check` — passed.
- Ruby `YAML.load_file` on `release.yml` — `macos` and `linux` need `check`;
  `release` needs `macos` and `linux`, runs only when
  `github.event_name == 'push'`, and alone has `contents: write`.

## Follow-up work

- Run `actionlint` on `.github/workflows/release.yml`.
- After the change reaches `main`, run `gh workflow run release.yml` and check
  that the three archives appear as artifacts and no release is created.
- The Arch run of `test-release` happens in CI.
