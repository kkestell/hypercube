# Builds and beta releases review

## Scope and coverage

Reviewed the requested build identity, application startup, Settings, packaging,
release scripts, workflows, installation guidance, and subsequent fixes in
`internal/desktop/desktop.go` and the app entry point. Unrelated changes were
outside scope. GitHub-hosted execution and a downloaded app's Gatekeeper and
Finder behavior were not exercised; the parent session is verifying the full
implementation.

## Summary

| ID | Severity | Category | Fix | Title |
| --- | --- | --- | --- | --- |
| F1 | Medium | Correctness | Fixed | Finder launch could use the wrong shell or fail |
| F2 | Medium | Testing | Fixed | Integration test rejected builds without VCS metadata |

## F1. Finder launch could use the wrong shell or fail

Medium · Correctness · Fixed · `cmd/hypercube/main.go:57`

Finder's environment need not contain `SHELL`. The original fallback always
selected zsh, even if the user's login shell was different, so its login
configuration and harness PATH could be missed. With `SHELL=/bin/tcsh`, the
original `-lic` invocation failed with `Unknown option: -lic`; its positional
argument syntax also assumed a Bourne-compatible shell.

The implementation now looks up the account's login shell when `SHELL` is
absent, selects login mode using the argument-zero convention, and passes the
executable in an environment variable. Isolated probes of zsh, bash, and tcsh
executed a path containing spaces with the expected arguments.

## F2. Integration test rejected builds without VCS metadata

Medium · Testing · Fixed · `cmd/hypercube/integration_test.go:585`

The test required a version beginning with `Hypercube v`, although the
implementation supports `Hypercube devel` without Git metadata. The focused test
failed with `GOFLAGS=-buildvcs=false` and `Hypercube devel`.

The assertion now accepts the development identity and retains the comparison
between the executable and server identities. The focused test passed with VCS
metadata disabled.

## Checks run

- `go test ./internal/buildinfo` passed during the initial review.
- `GOFLAGS=-buildvcs=false go test ./cmd/hypercube -run '^TestAppProcess$' -count=1` reproduced F2 before the fix and passed afterward outside the sandbox.
- A direct tcsh invocation reproduced F1; isolated zsh, bash, and tcsh probes passed after the fix.
- Confirmed `macos-15` maps to arm64 in GitHub's runner documentation.

Final implementation validation by the parent session:

- `make check` passed: generated assets, TypeScript, 342 frontend tests, Go
  vet, formatting, and Go race tests. Existing React `act` warnings remain.
- The app-process regression passed with and without VCS metadata. It now
  exercises zsh, bash, and tcsh login files in disposable homes, with a spaced
  executable path and no native window or real harness.
- Actionlint accepted both workflows. GitHub-hosted jobs and remote release
  creation were not run.
- An isolated Git fixture refused untagged and dirty releases, built a clean
  beta, and reported `+dirty` for an ordinary modified checkout build. After
  the shell fixes, the final fixture beta was packaged and extracted; the
  extracted app passed deep strict signature verification, reported the same
  identity as its bundle metadata, and matched the ZIP checksum. All four
  plugins and the existing service installer were included.
- The isolated development server served the page and current build identity;
  the browser view was opened for the user's visual assessment. No visual
  judgment, downloaded-app Gatekeeper approval, or live model test was made.
- No real-checkout tags, commits, staging, installations, or remote releases
  were made. The release tags existed only in a disposable test repository.

## Verdict

Both findings were fixed and their reported failures verified. No additional
findings in the corrections.
