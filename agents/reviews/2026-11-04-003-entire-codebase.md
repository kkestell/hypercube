# Entire codebase review

## Scope and coverage

Reviewed the first-party source and tests under `cmd/`, `internal/`, and
`plugins/`, both scripts, the Makefile, and build configuration at commit
`96d0992`. Three subagents reviewed the frontend; session, store, and workspace
packages; and harness plugins. The primary reviewer covered the remaining
packages and independently verified their reported reproductions. User
documentation supplied behavior requirements. Generated browser assets were
checked against a fresh build.

The user subsequently authorized fixes for F1–F11. All eleven findings are
fixed, with independent review by agents who did not write each implementation.
Changes remain unstaged and uncommitted. Source locations in the findings refer
to the original reviewed revision; the fixes describe the resulting code.

Gaps:

- No live model or installed harness integration was exercised. Harness tests
  and probes used stand-ins and isolated temporary state.
- No native application smoke test or graphical inspection was performed.
  The menu finding was verified through overlay state and native placement
  messages, not a screenshot.
- Third-party dependency internals and generated bundles were not manually
  audited. Generated asset freshness passed.
- F11 was established by tracing concurrent operations. Added regressions now
  cover terminal admission during family and child cleanup. Upgrade checks use
  isolated previous schemas, not the user's installed database.
- An older Codex private home with no input links or recorded source identity
  cannot recover a different selected account's source path. It retains its
  existing account state; matching alternate-home hashes can recover the source.
  New initially empty homes record their source so this ambiguity does not recur.

## Summary

| ID | Severity | Category | Fix | Title |
| -- | -------- | -------- | --- | ----- |
| F1 | High | Concurrency | Fixed | Family deletion can erase an unchecked child's work |
| F2 | High | Concurrency | Fixed | Ending a child removes its files before stopping its writers |
| F3 | High | Correctness | Fixed | Older saved databases no longer receive required schema upgrades |
| F4 | Medium | Correctness | Fixed | Codex sends child-question answers to the main agent |
| F5 | Medium | Correctness | Fixed | Invalid numeric answers can leave a Codex form stuck |
| F6 | Medium | Correctness | Fixed | Included Unicode paths prevent worktree creation |
| F7 | Medium | Correctness | Fixed | Resumed Codex conversations miss newly created shared inputs |
| F8 | Medium | Correctness | Fixed | Markdown entities remain encoded in text and link destinations |
| F9 | Medium | Concurrency | Fixed | Streaming in another pane replaces the active Find highlight |
| F10 | Medium | Correctness | Fixed | Profile and View menus can be obscured by the native browser |
| F11 | Medium | Concurrency | Fixed | A terminal can launch after its session finishes stopping |

## F1. Family deletion can erase an unchecked child's work

High · Concurrency · Fixed · `internal/session/sessions.go:260`

`removeFamily` reads its child list and checks the worktrees before it claims
deletion at line 317. `CreateChild` can add a child during those checks. The
later stop reaches that child, but the cleanup loop still uses the original
list. For a top-level worktree session, removing the parent worktree also
removes the new child's nested worktree without checking its uncommitted work.
Deleting the parent database row then cascades the child's row.

### Evidence

An isolated probe paused the parent's Git precheck, created a child with an
uncommitted file, then released deletion. `Delete` returned nil, the file was
gone, and reading the child's row returned `not found`.

### Fix

`removal.go` claims cleanup and captures membership under the same runtime
lock as child registration. Child creation refuses claimed families. Cleanup
checks the complete family again after its agents and shells stop, then records
each successfully removed child's completion before advancing. This preserves
consistent state if a later removal fails. Initial refusals leave normal message
delivery and agent relaunches working.

## F2. Ending a child removes its files before stopping its writers

High · Concurrency · Fixed · `internal/session/children.go:145`

`EndChild` calls `RemoveWorktree` before marking the launch as stopping or
stopping its agent and terminal. Those processes can write after Git reports
a clean worktree and before forced removal, losing the newly written work.

### Evidence

A second isolated probe paused cleanup after the real Git status operation
reported clean. The child's launch still had `stopping=false`. A file written
at that point was removed, and `EndChild` returned nil. This case requires no
concurrent child creation.

### Fix

`EndChild`, now in `removal.go`, claims cleanup, keeps the initial refusal
checks, stops the child's agent and shell, and lets `RemoveWorktree` check again
before removal. The child is marked ended only after successful cleanup. A
regression verifies that a shell's shutdown write is preserved and that cleanup
can be retried after the work is handled.

## F3. Older saved databases no longer receive required schema upgrades

High · Correctness · Fixed · `internal/store/migrate.go:6`

`CREATE TABLE IF NOT EXISTS` leaves an existing table unchanged. A database
created before top-level worktree sessions lacks `sessions.base`, but startup
no longer adds that column. Opening the database succeeds; reading sessions
then fails, preventing session lists and restoration from using the saved
conversations.

### Evidence

Commit `cc1ad20` used the schema without `base`. Commit `84e8317` introduced
the column and its upgrade; `ae4a9d9` removed that upgrade and older migrations.
An isolated database with the previous schema shape reopened successfully,
then `Session("s")` failed with `SQL logic error: no such column: base (1)`.

The current saved-state requirement says state carries between versions, and
the installation guide still promises upgrades from the earlier schema.
The deletion commit's description calls the migrations obsolete; no current
instruction establishes an exception to those upgrade requirements.

### Fix

Restored the known agent-table conversion and launch/base column upgrades
inside the existing migration transaction. Restored the original schema fixture
and preservation tests, split by schema generation. They verify saved sessions,
conversations, transcript ordering and senders, queued messages, repeat opens,
and refusal of an unconvertible legacy database.

## F4. Codex sends child-question answers to the main agent

Medium · Correctness · Fixed · `plugins/codex/requests.go:440`

Async questions retain their originating native thread, including a native
subagent's thread. `answerAsync` instead calls `ag.input`, which always uses
the main thread and turn. The main agent receives the answer while the child
gets none, and Hypercube closes the child's question.

### Evidence

An in-process Codex probe emitted a child async question and answered it. The
outgoing `turn/steer` request named `threadId:main` and
`expectedTurnId:main-turn`, rather than the originating child.

### Fix

Async answers reuse input delivery with an explicit originating thread. Known
native children track their current/completed turn alongside their transcript
parent. Child turns retain their native settings and do not change the main
agent's state. Stand-in regressions cover active, idle, and just-ended child
turns and keep unrelated main-thread questions open.

## F5. Invalid numeric answers can leave a Codex form stuck

Medium · Correctness · Fixed · `plugins/codex/requests.go:198`

`formValue` accepts the nonfinite values accepted by `strconv.ParseFloat`.
In a form with several fields, answering an early numeric field with `NaN`
marks it complete. Answering the final field then fails JSON encoding. The
completed field cannot be corrected, so the form cannot finish. The same
conversion also accepts fractions for fields declared as integers.

### Evidence

An isolated two-field form accepted `NaN` and completed the numeric field.
The final answer failed with `json: unsupported value: NaN`; correcting the
first answer failed because its request was no longer open. A separate probe
confirmed that an integer field accepted `1.5`.

### Fix

`formValue` now rejects nonfinite numbers and fractional integer values before
a field is completed. Form regressions verify rejection, correction, and
successful final submission.

## F6. Included Unicode paths prevent worktree creation

Medium · Correctness · Fixed · `internal/workspace/workspace.go:229`

`copyIncluded` handles newline-delimited, quoted Git output as literal
filesystem paths. With default `core.quotePath`, a valid ignored Unicode
directory selected by `.worktreeinclude` is quoted, and the subsequent copy
looks for the quoted spelling. Creating either a top-level or child worktree
fails.

### Evidence

An ignored and included `café/` directory caused `AddWorktree` to fail with
`copying "caf\303\251/": cp: ... No such file or directory`.

### Fix

Both Git commands now use NUL-delimited input/output through the existing
untrimmed command runner. Worktree regressions cover Unicode files/directories,
quotes, tabs, line breaks, and leading spaces.

## F7. Resumed Codex conversations miss newly created shared inputs

Medium · Correctness · Fixed · `plugins/codex/storage.go:55`

Finding an existing private conversation returns before reconciling the
shared inputs. If a source input did not exist at initial setup, creating it
later does not make it available when that conversation resumes. For example,
installing the first skill into the original source home remains invisible
to resumed Hypercube conversations.

### Evidence

An isolated storage probe prepared a private home without a source `skills`
directory, saved a conversation, created a skill, and resumed. The skill was
missing from the resumed home. Starting a new conversation against the same
source created the missing link and exposed the skill in that same home.

### Fix

Resumes recover the original source from existing input links and reconcile
newly available inputs before returning the private home. Initially empty
sources retain a directory link as provenance; older alternate homes can be
recovered only when the selected source matches their saved hash. Storage
regressions verify newly installed skills and account retention even when the
current account selection differs or is invalid.

## F8. Markdown entities remain encoded in text and link destinations

Medium · Correctness · Fixed ·
`internal/ui/web/transcript/markdown.tsx:85`

The lexer preserves Markdown entities, and React escapes those strings again.
Entities appear literally in chat and Markdown previews. In link destinations,
the preserved entity also changes query parameters.

### Evidence

Rendering `Tom &amp; Jerry` produced `Tom &amp;amp; Jerry` in serialized HTML.
Rendering `[link](https://example.com/?a=1&amp;b=2)` produced an href containing
literal `&amp;` query text, resulting in parameter `amp;b` instead of `b`.
The link path passes through lines 79 and 21 of the same file.

### Fix

Markdown text and link/image attributes now use the existing installed
`entities` decoder, promoted to a pinned runtime dependency. Code and raw HTML
stay literal, and decoded destinations still pass URL validation. DOM tests
cover text, compact previews, links, images, and unsafe decoded destinations.

## F9. Streaming in another pane replaces the active Find highlight

Medium · Concurrency · Fixed · `internal/ui/web/shared/find.tsx:64`

Every mounted Find writes and deletes the document-wide `find` highlight,
regardless of which pane is active. With Find open in two panes, streaming
content in the inactive pane replaces the active pane's current highlight.
Closing another Find can also delete the active highlight.

### Evidence

A focused DOM probe searched for `text` in pane 1, then focused pane 2's Find
and searched for `some`. Appending content to pane 1 changed the global
highlight from `some` to `text`, while keyboard focus remained in pane 2's
Find field.

### Fix

Only an active, open Find publishes the shared highlight. Cleanup removes it
only when it still owns that exact highlight. The two-pane regression covers
inactive streaming, pane changes, and closing either search.

## F10. Profile and View menus can be obscured by the native browser

Medium · Correctness · Fixed ·
`internal/ui/web/agent/profile-select.tsx:46`

The Profile dropdown and `internal/ui/web/editor/view-menu.tsx:23` do not
register their content with the existing overlay tracker. In a split layout,
an overlapping session browser remains placed above the menu and obscures
its options. Context menus already register their overlays.

### Evidence

A DOM probe mounted a browser and Profile selector with intersecting bounds,
then opened the menu. The menu existed, but `state.overlays` remained empty
and no browser-hide message was sent. `useBrowserPlacement` hides the native
view only for registered overlaps; the native code places that view above
the page.

### Fix

Both dropdown contents now use the existing `useOverlay` ref. DOM regressions
verify that opening either menu hides an overlapping native browser and closing
the menu restores its placement.

## F11. A terminal can launch after its session finishes stopping

Medium · Concurrency · Fixed · `internal/session/sessions.go:449`

`Shell` checks the session's running state in a completed database transaction,
then calls `Terminals.Open` without lifecycle synchronization. A concurrent
Stop or EndChild can finish its shell cleanup between those operations.
The delayed request then starts a shell for the stopped session.

### Evidence

The read ends at line 445 and shell admission occurs at line 449. Stop marks
the family stopped and calls `Terminals.Stop`; EndChild similarly ends the
child and calls `Terminals.Stop`. Neither operation prevents a previously
checked request from subsequently reaching `terminal.Set.Open`. That method
checks only the set-wide `closed` flag used by Quit. This finding is confirmed
by the operation ordering rather than a runtime probe.

### Fix

`Shell` holds the existing runtime mutex through its running/cleanup checks
and holder creation. Stop/end/delete publish their lifecycle changes under the
same mutex and wait for processes outside it. No additional mutex or terminal
state was introduced. Cleanup regressions verify rejected shell admission.

## Checks run

- Initial review: `make check` passed program builds, asset comparison, TypeScript,
  all 335 frontend tests, Go vet, formatting, and all Go race tests. Frontend
  tests emitted React `act(...)` warnings.
- Temporary Go overlays added focused probes without editing checkout files.
  The primary reviewer reran the probes for family deletion, EndChild cleanup,
  the missing schema upgrade, Unicode worktree inclusion, Codex child-question
  routing, numeric forms, and resumed shared inputs. Their expected-behavior
  assertions failed against the originally reviewed code, confirming F1–F7.
- The three frontend probes were rerun using the existing DOM/render helpers:
  Markdown serialization, two-pane Find with an inactive streaming update,
  and menu overlay/native placement messages. They confirmed F8–F10.
- The workspace reviewer also checked a conflicted Git index and broad `*`
  worktree inclusion; those checks passed.
- After the authorized fixes, final `make check` passed with 342 frontend tests,
  generated-asset freshness, TypeScript, Go vet, formatting, and all Go race
  tests. React's existing `act(...)` warnings remain in the frontend suite.
- Independent implementation reviews found one partial-cleanup regression;
  per-child persistence was restored and the targeted Delete/Archive failure
  and retry regressions passed the reviewer's recheck.
- A final-check failure exposed an existing timing flaw in `TestArchiveFamily`:
  it captured the transcript before the expected child reply. Its setup now
  waits for that reply; eight focused race-test repetitions and the final full
  check passed.
- `make dev` served the rebuilt browser bundle on the existing isolated dev
  address. Page/profile routes passed, served JavaScript matched the generated
  bundle, and the browser was opened for the user's assessment. The smoke-test
  process was cleaned up. No real harness sessions were started.
- The final diff was inspected. The Git index and installed app are unchanged.

## Verdict

All three High and eight Medium findings are fixed. Independent implementation
reviews have no remaining findings, and the final repository checks pass.
The Codex plugin corrections require active sessions to restart after
installation. Changes remain unstaged and uncommitted.
