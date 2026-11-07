# Linux test environment

## Plan

`agents/plans/2026-11-06-014-linux-test-environment.md`

## Summary

Added `scripts/linux/Dockerfile`, the executable `scripts/linux/run`, the
`linux-check` Makefile target, and a "Test on Linux" section in
`agents/development.md`. The goal is met: `scripts/linux/run` passes `make
check`, `make dev` serves the page, Electron runs under Playwright, and the
systemd user manager reports `running`.

## Departures from the plan

- The plan's boot wait, `systemctl is-system-running --wait`, failed when it
  ran before systemd's bus existed ("Failed to connect to system scope bus"),
  and `run` aborted. `run` now retries until `systemctl` prints a state, then
  requires `running`.

## Decisions

- Bun's zip is unpacked with `unzip -j '*/bun'`, so the archive's folder name
  does not matter.

## Checks run

- `scripts/linux/run` (no image, no cache volume) — passed; 2 min 46 s
  including the image build.
- `scripts/linux/run` again — passed in 11.8 s; a third run took 11.4 s.

## Manual verification

1. `make dev` in the container.

   ```sh
   scripts/linux/run make dev
   docker exec <container> curl http://localhost:4281/
   ```

   The page's HTML came back. Ctrl-C, sent through a pseudo-terminal, exited
   with status 130 and left no container
   (`docker ps -a --filter ancestor=hypercube-linux`).

2. Electron under Playwright, through a script passed to `scripts/linux/run
   bash -c`, not committed. It ran `bun install` and
   `node node_modules/electron/install.js` in `desktop`, launched Electron with
   `_electron.launch`, and read `process.platform`.

   Printed `linux`; the screenshot shows the rendered text. The run took 5 s.

3. Systemd user manager.

   ```sh
   scripts/linux/run sh -c 'systemctl --user is-system-running'
   ```

   Printed `running` in three consecutive runs.

4. `git status --short --ignored` was unchanged by the runs: no ignored files
   appeared in the checkout. No trial containers remain; the `hypercube-linux`
   image and `hypercube-linux-cache` volume are kept.
