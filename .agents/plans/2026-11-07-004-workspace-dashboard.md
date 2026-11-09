# Workspace dashboard

## Goal

Clicking a workspace heading opens a dashboard in the same tabbed area as a
session. The first dashboard contains recent sessions. Files, diffs, search,
Quick Open, and the terminal work against the workspace's main checkout and
whatever branch is checked out there. Selecting a session restores its usual
view. Keep this first version small; there are no additional dashboard widgets.

## Related code

- `internal/ui/web/sessions/selection.ts`, `types.ts`, and `view-state.ts`,
  `internal/ui/web/state.ts` — selection, session attachment, and remembered
  tabs, files, diffs, scroll, and terminal visibility currently share one
  session object.
- `internal/ui/web/editor/` — the common tab layout, file/diff views, and the
  agent tab; `sessions/session-view.tsx` chooses whether to render it.
- `internal/ui/web/sidebar/workspace-section.tsx` — workspace headings
  currently collapse their session lists.
- `internal/ui/web/files/`, `changes/`, `api.ts`, and `app/` — file tools,
  polling, request parameters, panel state, shortcuts, title, and URL selection.
- `internal/ui/web/data-socket.ts`, `transcript/transcripts.ts`, and
  `app/{view-storage,saved-views}.ts` — real session subscriptions and persisted
  browser view choices.
- `internal/ui/{files,changes,mention_files,search_files,previews,terminal}.go`
  — file and terminal endpoints; mention files already accepts a workspace.
- `internal/session/{sessions,restore,runtime}.go` and
  `internal/terminal/terminal.go` — shell admission, ownership, restoration,
  and cleanup. The terminal set already accepts a string owner.

## Decisions

### One shared editor attachment, real session identity kept separate

Extract the shared tabs, active pane, files, changes, diffs, scroll, and panel
visibility from the attached session into a current editor view. Its target is
explicitly either a real session ID or a workspace ID; its stable key
distinguishes the two. Keep the transcript, transcript readiness, and expanded
transcript items in the real session attachment. There is no saved or synthetic
session for a dashboard, and opening it does not start, wake, or mark an agent
seen.

Use the existing `selected` string for navigation: existing raw session IDs
keep their meaning; `workspace:<id>` names a dashboard. Session IDs are generated
without that prefix. Centralize decoding and target/request parameters instead
of checking this prefix throughout components. `currentSession` continues to
mean a real selected session. The URL and page title support both selections.
New Session opened from a dashboard uses that workspace. Do not change initial
selection behavior elsewhere.

Attachment owns the existing request-generation checks and transient file-tree
cache, now keyed by view identity. Switching between dashboard and session
parks unfinished prompts and preserves each view's tabs, splits, folders,
scroll, and terminal visibility. A late read or departing view's scroll must
not update another attachment. Session-directed file-open messages continue to
queue for their real session, including while a dashboard is shown. The data
socket announces an empty session while the dashboard is selected.

Keep the existing saved session-view format readable, including its earlier
terminal-visibility migration. Add optional saved workspace-view choices with
an empty default for old data; use the common view choices without transcript
expansion. Saved data remains browser view state, with no database migration.
Remove dashboard choices/cache when the workspace disappears. Losing the
selected workspace clears its view using the existing empty-view behavior.

### Dashboard tab and sidebar navigation

Give the shared editor a dashboard tab kind beside agent, file, and diff. A
workspace starts with its dashboard tab; it is kept open like the existing
agent tab. Files and diffs retain the current preview and split behavior.

The dashboard shows the workspace name and a Recent Sessions list. Use up to
ten top-level sessions from the existing newest-first workspace snapshot,
including archived sessions, with their existing status conventions and links
through normal session selection. Child sessions remain under their parents in
the sidebar. Show a short empty state when there are no sessions. No timestamps,
new activity ordering, session query, or database fields are needed.

Clicking the workspace name selects its dashboard and indicates that selection.
A separate, labeled chevron button collapses or expands its session list;
keyboard users can reach both controls. Preserve New Session, workspace menus,
and independent collapse state. Use existing stock components and styles.

### Resolve the real directory once for every file tool

Share the concrete session/workspace directory resolution already present in
`mentionRoot`; use it for file listing, reads, changes, diffs, Quick Open, and
text search. The browser sends either `session` or `workspace` parameters, never
a synthetic session ID or arbitrary directory. Preserve existing session file
access, path validation, limits, and errors.

Workspace reads use `store.Workspace.Path`, never a recent session's directory
or managed worktree. The current checkout's branch and working tree therefore
follow normal existing polling. Non-Git workspaces retain file tools without
Git changes.

Extend preview routing with a distinct workspace prefix while retaining the
existing session preview route. Keep absolute file paths after that prefix so
relative HTML resources still resolve. Both preview routes validate that their
real owner exists and retain existing content sandboxing and media behavior.
Path context menus derive their absolute paths from the current target's real
directory. Search-result line navigation opens a file in the current view;
keep agent-requested session file opens separate.

### Reuse the terminal's existing lifecycle

Support the usual terminal panel on the dashboard. Add a workspace shell entry
point to the existing runtime and a distinct workspace owner key in the same
terminal set. It starts lazily in the workspace path, stays running while the
panel is hidden or another view is selected, and is stopped when the workspace
is deleted or the app quits. Use the existing lifecycle lock and workspace
deletion claim to refuse shell admission during deletion and shutdown.

Include existing workspaces, even those with no sessions, in restoration's
known shell owners and adoption. Stop the workspace shell only after session
deletions succeed and before removing the workspace record. Retain the existing
session shell behavior and holder format. This needs no new shell manager,
process supervisor, persistent owner table, or agent launch changes.

## Implementation plan

1. Extract the common editor view and target helpers. Adapt attachment,
   session-view snapshots/restoration, browser storage, and selection together,
   retaining old saved session views and the existing unsent-session behavior.
   Separate file-opening in the current view from agent requests targeting a
   real session. Move common operations to their editor/file owners instead of
   adding dashboard branches throughout session code.
2. Generalize file API parameters and backend directory resolution, then update
   files, changes, polling, previews, search, Quick Open, and path menus to use
   the shared target. Preserve session endpoint behavior and response formats.
3. Add the dashboard tab/view and workspace-heading navigation with a separate
   collapse button. Update the app frame, panel keys, title/hash, New Session
   workspace default, shortcuts, and layout toggles for the shared view. Keep
   transcript subscription, seen handling, composer, and session actions gated
   by real session identity.
4. Extend the existing terminal route/client and runtime shell ownership for
   workspaces, including admission, restore, deletion, and quit. Split shell
   helpers out of `sessions.go` if that keeps the whole file easier to follow.
5. Add focused behavior and regression coverage below, update the narrow guide
   references, and run `make check`. Write/stage the work log and task changes
   for the required separate review. The parent commits validated work and
   reuses or starts `make dev`, then opens its URL for user testing.

## Test plan

- Browser behavior: workspace-heading click shows recent sessions in existing
  order; empty state; clicking a recent archived or active session opens it;
  the separate chevron changes collapse state without navigating.
- Selection/state: session → workspace → session preserves tabs, splits,
  scroll, file choices, transcript expansion, and unsent prompt state; New
  Session uses the dashboard workspace; workspace deletion clears its selected
  view and remembered choices. Old saved views load, and workspace selection
  and view choices restore after reload.
- Shared file behavior: a workspace file and diff open in tabs, search results
  reveal matching lines, and Quick Open requests the workspace. At the existing
  file-polling boundary, prove stale responses cannot cross session/workspace
  attachments, including switching away and back. Session-directed file opens
  remain queued while the dashboard is selected. Socket coverage verifies no
  workspace ID is sent as a session subscription.
- Backend routes through the existing test server: workspace file listing,
  read, mention files, search, changes, and diff use its main checkout when a
  recent session uses another directory/worktree. Switching the checkout's
  branch updates reported changes without changing the dashboard target.
  Missing owners are refused; existing session route tests remain green.
- Preview regression: workspace media/HTML preview and an HTML relative
  resource load through the workspace prefix; session previews keep working.
- Terminal boundary: a workspace shell starts in its workspace, survives a
  reconnect and runtime detach/restore, is refused during workspace deletion,
  and is stopped on successful deletion and quit. Preserve existing session
  shell tests. Reuse isolated test shells and state.
- Run focused Go and Bun tests while working, then `make check` once the final
  code is ready. Use tests and the build for browser validation; no screenshots.
  Leave the requested dev UI available for the user to test.

## Documentation updates

- `docs/interface.md`: workspace-heading navigation and separate collapse
  control; workspace checkout scope for file tools; per-workspace saved view
  choices. Keep the existing guide structure and screenshots.
