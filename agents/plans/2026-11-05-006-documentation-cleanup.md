# Separate user and developer documentation

## Goal

Keep user documentation in `docs/` and developer reference in `agents/`, with
root `AGENTS.md` as the agent entry point. Delete obsolete material; Git
history preserves past designs and work. GitHub Issues owns outstanding work.

## Related code

- `README.md`, `AGENTS.md` — documentation entry points and development rules.
- `docs/` — five user guides plus the architecture guide to move.
- `agents/` — research, designs, outstanding work, and dated task records.

## Decisions

- Reuse the five user guides. Move architecture and extract development
  reference; do not add `agents/README.md` or an archive directory.
- Keep user-facing explanations of sessions, worktrees, harness differences,
  storage, and compatibility. Put implementation details in developer reference.
- Retain research and task records only when useful to current development or
  active work. Do not preserve obsolete files by marking them superseded.
- Before transferring any finding to GitHub Issues, verify its relevance
  against the current implementation and supporting evidence, then check for
  an existing issue. Create issues only for verified findings not already
  tracked. This cleanup does not include fixing those findings.
- Preserve concurrent work. In particular, do not delete another session's
  active plans, reviews, work logs, or handoffs as completed history.

## Implementation plan

1. Move `docs/architecture.md` to `agents/architecture.md`. Incorporate the
   plugin-protocol section from `docs/harness-support.md`; leave requirements
   and user-visible harness behavior in the user guide.
2. Extract development and release instructions from `docs/installation.md`
   into `agents/development.md`. Keep user installation, upgrades, service
   management, backup, recovery, build identification, and a short
   source-install recipe in the installation guide. Link to existing agent
   rules rather than duplicating them.
3. Update `README.md` to index user guides and direct developers to
   `AGENTS.md`. Update `AGENTS.md`'s directory descriptions and link directly
   to the two developer references, keeping instructions concise.
4. Verify and reconcile remaining items from `agents/roadmap.md` and
   `agents/issues.md` with GitHub Issues, then delete both files and their
   navigation links. Delete `agents/mux-design.md`.
5. Inspect `agents/research/`, `plans/`, `work/`, `reviews/`, `scratch/`, and
   `mockups/`. Preserve still-useful information in its current reference or
   a verified GitHub issue, then delete obsolete, redundant, or completed
   records and disposable artifacts. Keep material needed by active work.
6. Trim duplicated and implementation-specific prose from the user guides.
   Remove personal setup commentary. Replace machine-specific dependencies
   with repository-relative links, portable instructions, or permanent
   external references. Preserve legitimate product paths. Leave the paired
   configuration example and actual user configuration unchanged.
7. Repair retained links after moves and deletions. Keep `.agents/skills/`
   in its discovery location. Do not rewrite historical records to describe
   current behavior or create replacement history documents.

## Test plan

- Inspect the documentation diff and retained links, file references, and
  anchors. Confirm instructions do not depend on private checkouts or
  disposable local files, and both audiences have direct entry points.
- Confirm migrated findings have current evidence and no duplicate issue.
- Documentation-only changes require no build or code checks.
