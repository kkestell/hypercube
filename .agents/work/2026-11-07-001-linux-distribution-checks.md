# Linux distribution checks

## Plan

`agents/plans/2026-11-07-001-linux-distribution-checks.md`

## Summary

`scripts/linux/Dockerfile` builds from `BASE`, and the new
`scripts/linux/packages` installs each distribution's packages. `scripts/linux/run`
reads `LINUX_DISTRO`, tags `hypercube-linux-<distro>`, and writes output to
`bin/linux-test/<distro>/`. The `Makefile` has `LINUX_DISTRO` and
`linux-test-all`, `check.yml` runs `linux-test` as a matrix over the four
distributions, and `agents/development.md` describes them. The Mac side of the
goal is met for Debian, Ubuntu, and Fedora on ARM64. Arch and the AMD64 runs
wait for CI.

## Decisions

- The Dockerfile removes the base image's user with UID 1000 whether or not one
  exists, with a comment saying why.
- `Makefile` exports `LINUX_DISTRO` so `scripts/linux/run` sees it when
  `linux-test-all` or `make linux-test` is called without the variable set on
  the command line.
- `packages` also cleans the package manager's cache (`dnf clean all`,
  `pacman -Scc`) to keep the images small; the plan did not mention it.

## Checks run

- `make linux-test-all` on the Mac (ARM64) — Debian, Ubuntu 26.04, and Fedora
  45 passed `test-bundle`, `test-install`, and `test-desktop`. Each
  `bin/linux-test/<distro>/` has the 10 screenshots and two logs.
- `LINUX_DISTRO=arch scripts/linux/run true` — failed at once with "Arch Linux
  publishes AMD64 images only".
- `LINUX_DISTRO=centos scripts/linux/run true` — failed with the list of names.
- `make linux-check` (Debian) — passed.
- `scripts/linux/test-apparmor` — passed.
- `sh -n scripts/linux/packages` — passed.
- `docker rmi hypercube-linux` — removed the old image; the per-distribution
  images and the cache volume remain.

## Manual verification

1. Viewed every screenshot of each distribution in `bin/linux-test/`: window,
   keys-menu, notification, workspace-menu, keys-shortcuts, reinstall, and the
   four waiting pages. Text renders and is readable in all three, the menus
   show Ctrl labels, and the dunst notification appears. Fedora's panel terminal shows bash's default prompt.

## Follow-up work

- Run the CI matrix. The Arch package list and the Arch run are unverified until
  then; inspect the `linux-test-arch` artifact's screenshots.
- Tick "Distribution checks" in `agents/todo.md` once all four pass.
