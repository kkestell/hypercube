# Workspace dashboard plan review

## Scope

Reviewed `agents/plans/2026-11-07-004-workspace-dashboard.md` against the
request and the existing selection, editor, file routes, socket, saved views,
and terminal lifecycle.

## Assessment

No blocking findings. The simpler baseline is a dashboard in the existing
editor with the workspace path as its file target. The plan follows that
baseline: it reuses the current session list and editor tools, adds no database
fields or agent session, and adds no dashboard widgets beyond recent sessions.

Separating shared editor state from transcript state is warranted. Reusing a
fake attached session would also enter transcript subscriptions, agent wakeup,
and queued session file opens. The shared attachment must remain a concrete
session-or-workspace target, not a framework for other target kinds.

The terminal extension fits the existing terminal set. Workspace deletion and
restoration need to recognize its owner because those paths already control
shell admission and remove unknown holders. They do not require a new shell
manager or stronger lifecycle guarantees.

The test plan identifies meaningful boundaries; implementation should reuse
existing coverage and add focused regressions rather than repeat every shared
file behavior at every layer. Preserve old saved view formats and session URLs.

## Verdict

Proceed with implementation. Keep the dashboard to recent sessions and the
existing companion tools.
