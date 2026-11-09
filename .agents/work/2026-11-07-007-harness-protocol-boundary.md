# Harness protocol and plugin client packages

## Plan

[Harness protocol boundary](../plans/2026-11-07-007-harness-protocol-boundary.md)

## Summary

The plan's goal is met: `internal/harness` holds protocol data and validation,
with no Hypercube package imports. `internal/harnessclient` owns discovery
execution and the holder-backed plugin client; session callers use it. Record
decoding is exported as `harness.DecodeRecord`. Runtime behavior is unchanged.
There were no departures from the plan.

## Checks run

- `go build ./...` — passed with a temporary writable Go build cache.
- `make check` — passed: browser and website builds, TypeScript, 368 browser
  tests, 28 desktop tests, Go build, vet, formatting, and all Go tests with
  the race detector, including record decoding and discovery cancellation.
- `go list` dependency checks — passed: `harness` imports only the standard
  library; the plugins and store depend internally on only `harness` and
  `store`; `harnessclient` depends internally on only `harness` and `hold`.
- Search for former `harness.Client`, `Launch`, `Attach`, `Plugins`, `Find`,
  `Describe`, and `Models` references — no matches.
- Compared the moved client and discovery execution with their originals —
  unchanged apart from package, qualified types, decoder name, and the
  `Models` parameter name needed to avoid shadowing the protocol package.
- `git diff --check` — passed.

## Manual verification

No live app or installation check was needed for this source extraction.
Existing process tests cover launch, attachment, delivery, and discovery
cancellation. Linux packaging and full runtime integration remain in the later
integration plan.
