# Review of mux IDs, select, server start, and pane attention

## Scope and coverage

Reviewed the staged changes for `agents/plans/2026-10-30-010-mux-ids-select-and-attention.md`: `mux/client/client.go`, `mux/cmd/mux/main.go`, `mux/cmd/mux/attach.go`, `mux/internal/server/server.go`, `label.go`, `pane.go`, the tests in `mux/cmd/mux/main_test.go`, and the `agents/mux-design.md` table row. Traced `selectTarget` against the title click in `mouse.go`, `setCollapsed`, and `layoutSession`; `Dial` and `DialOrStart` against `ListenAndServe`'s stale socket handling; and checked `ErrNoServer` and `DialOrStart` against what Milestone 2 piece 4 in `agents/mux-design.md` needs to replace `internal/tmux`.

Gaps:

- No live smoke test in `make dev`; the mux is not yet used by Hypercube.

## Summary

No findings.

The client API fits piece 4: Hypercube can call `Dial(SocketPath())` for lookups, kills, labels, attention, and select, and read `errors.Is(err, ErrNoServer)` as "no saved pane is running"; it calls `DialOrStart` with the mux executable only to spawn. `SocketPath` reads `XDG_STATE_HOME`, so Hypercube's isolated tests reach their own server through the environment. Nothing in the change is speculative.

## Checks run

- `make check` — passes.
- `go test -race -count=1 ./...` in `mux/` — passes.

## Verdict

The change meets the plan and is ready to commit.
