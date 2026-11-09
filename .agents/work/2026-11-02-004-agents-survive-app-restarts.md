# Agents and panel shells survive app restarts

## Plan

`agents/plans/2026-11-02-006-agents-survive-app-restarts.md`

## Summary

Implemented detached holders, committed output positions, durable commands,
reattachment and recovery, panel shell adoption, MCP restart retries, and the
separate Quit and Detach paths. The plan's goal is met with isolated stand-in
harnesses and real app process tests.

## Departures from the plan

- Command recording uses the existing runtime mutex; a launch's send mutex
  serializes snapshots and writes in database ID order. This preserves the
  specified ordering while letting the output reader record its next delivery
  without waiting for a blocked input write.
- Shell attachments wait for their active output readers to finish before
  removing an ended holder's files, preserving final output for WebSockets.
- Added `TestMain` and short state directories to the MCP and UI tests too,
  because those existing fixtures also launch agents through holders.

## Decisions

- Delivery timeout notes use the command ID as their item ID, so attaching
  again cannot create a second copy of the same note.
- The stand-in has explicit append, completion, acceptance-on-close, and
  held-start cases for the restart and stop regressions; no live harnesses are
  used.

## Checks run

- `make check` — passed: all builds, generated assets, 158 Bun tests, Go vet,
  formatting, and the full Go race suite. Go build, module, and sum database
  caches were isolated under `/private/tmp`; the existing Go 1.27.1 toolchain
  was selected through PATH with `GOTOOLCHAIN=local`.
- Focused holder, harness, terminal, session, MCP, and app tests — passed.
  The holder tests include killed-parent survival, partial and concurrent
  lines, 8 MiB lines, rotation, lost holders, forced stop, PTY hangup, and resize.
- `TestAppProcess` — passed: agents and a panel shell retained their process
  IDs and output through SIGTERM and SIGKILL, ended on SIGINT, and resumed
  awaiting the user after Quit.
- Runtime restart tests — passed, including the real one-minute start timeout
  and 30-second delivery timeout, streaming replay, attention, unsent and held
  commands, offline exits, quick-exit counts, lost families, and orphan cleanup.
- `bash -n scripts/dev` and `git diff --check` — passed.

No installed service, user workspace, or live harness was used.
