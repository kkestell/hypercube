# Linux install and service

## Plan

`agents/plans/2026-11-06-016-linux-install-and-service.md`

## Summary

`make install`, `make install-service`, and `make uninstall` work on Linux
through `scripts/linux-install`, `scripts/linux-install-service`, and
`scripts/linux-uninstall`, which the bundle ships as `resources/install`,
`install-service`, and `uninstall`. The AppArmor profile step is in the
installer. `scripts/linux/test-install` runs in `make linux-test`;
`scripts/linux/test-apparmor` checks the profile in a Lima VM. The Dockerfile
gained `zsh` and `iproute2`, `test-bundle` checks the three scripts, and
`agents/development.md` describes the scripts and tests. The plan's goal is
met.

## Departures from the plan

- The AppArmor test picks the sandboxed zygote with `pgrep -f -- '--type=zygote$'`.
  Electron starts two zygotes: one with `--no-zygote-sandbox`, which shares the
  main process's user namespace, and the sandboxed one.
- `test-apparmor` starts the VM with `--mount-none`, so the VM cannot read the
  Mac's home directory.
- `test-install` sends an `Origin` header with its `POST` requests, which the
  backend requires.
- `linux-install` links every `hypercube-harness-*` in the bundle instead of
  naming the four plugins.

## Checks run

- `make linux-test` — passed: `test-bundle` and `test-install`.
- `scripts/linux/test-install` with `KillMode=control-group` in the unit —
  failed with "the holder did not survive the restart"; restored.
- `scripts/linux/test-apparmor` — passed on an Ubuntu 24.04 arm64 VM
  (`vmType: vz`): the sandbox aborted without the profile, worked with it, and
  uninstalling removed the file and unloaded the profile. `limactl list` shows
  no instance afterward.
- `make check` — passed.
- `make -n install install-service` on macOS — prints the macOS commands;
  `make/macos.mk` and `scripts/install-service` are unchanged and the
  `Makefile` change is the `linux-test` command only.
- `shellcheck` is not installed; it was not run.

## Follow-up work

- `scripts/linux-install` re-run with the profile already in place skips `sudo`
  by comparing file contents; the AppArmor test does not install twice to
  check that.
- The CI `linux-test` job has not run with `test-install`; it runs on the next
  push.
