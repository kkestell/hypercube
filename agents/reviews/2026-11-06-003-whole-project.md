# Whole-project code review

Reviewed commit `443adb99e2cbabf092de719e84adaf9fa0cba2cc` on November 6,
2026. Four GPT workers reviewed runtime/persistence, plugins/transports,
frontend, and platform/build concerns. The parent checked their conclusions,
followed the important paths, and independently reran the strongest probes.
This review changes no application code.

The biggest wins are completing ownership of concurrent lifecycle operations,
keeping managed worktrees out of initialization copies, and giving file reads
one owner. The existing process, persistence, and native-plugin boundaries
generally serve real requirements. There is limited evidence for a broad
architectural rewrite.

## Findings, biggest wins first

Ordering weighs consequence, reach, and likely benefit relative to change
cost. P2 denotes a concrete correctness or bounded-work problem; P3 denotes
a smaller improvement. Confidence and verification limits appear below.

| Rank | Priority | Finding | Main location |
| --- | --- | --- | --- |
| 1 | P2 | Workspace deletion can erase a newly admitted session without cleaning it up | `internal/session/sessions.go:88` |
| 2 | P2 | Worktree initialization copies managed worktrees into the new worktree | `internal/workspace/workspace.go:243` |
| 3 | P2 | Overlapping Codex inputs let a new turn escape an interrupt | `plugins/codex/main.go:271` |
| 4 | P2 | Deleting a session can permanently stop all desktop alerts | `internal/session/alerts.go:67` |
| 5 | P2 | File-opening and polling reads can replace fresh content with stale content | `internal/ui/web/files/files.ts:108` |
| 6 | P2 | Service installation emits invalid XML for valid filesystem paths | `scripts/install-service:42` |
| 7 | P2 | A bounded MCP transcript page loads all history under the global runtime lock | `internal/mcp/read_session.go:37` |
| 8 | P2 | Directory-listing failures disappear into an empty or stale tree | `internal/ui/web/files/files.ts:98` |
| 9 | P2 | Public feature documentation describes capabilities Hypercube does not own | `web/content/features.md:18` |
| 10 | P3 | Streamed transcript updates repeat ordering and row projection work | `internal/ui/web/transcript/transcripts.ts:15` |

### 1. Claim workspace membership before deleting its sessions

**High confidence; reproduced through public runtime operations, including
under the race detector.**

`DeleteWorkspace` reads the current top-level sessions, deletes each, and
finally deletes the workspace. `Create` independently reads that workspace,
creates its optional worktree, registers the session, and starts input
(`internal/session/sessions.go:55`, `:74`, `:83`). Nothing prevents registration
after deletion takes its membership snapshot. The final workspace deletion
cascades session rows through SQLite.

The isolated probe paused deletion during an existing worktree's removal
check, created another worktree session in the same workspace, and wrote an
uncommitted file there. Deletion then returned successfully. The late session's
database row and transcript were gone, while its dirty worktree and live
runtime attachment remained. Subsequent cleanup logged
`recording the end of <ID>: session "<ID>": not found`.

This bypasses the project's actual worktree safety and process cleanup
responsibilities. The existing family-removal claim in
`internal/session/removal.go:85` already recognizes the same membership
problem for child creation.

**Direction:** claim workspace deletion under the runtime mutex before taking
the snapshot, and make top-level admission check that claim under the same
ownership. Cover in-progress creation through registration or cleanup, and
keep filesystem/Git work outside the mutex. Release the claim when removal
finishes or refuses. Preserve the dirty-worktree refusal. A generalized
deletion framework or durable deletion journal is unnecessary.

### 2. Keep managed worktrees out of inclusion copies

**High confidence; reproduced with real Git and macOS `cp` in an isolated
repository.**

`AddWorktree` adds `/.hypercube/` to Git's local excludes before creating the
new worktree. `copyIncluded` then selects paths matching `.worktreeinclude`,
keeps the ignored paths, and recursively copies them. It never excludes the
managed worktree directory itself.

With a valid `.worktreeinclude` containing `*`, the actual selection commands
returned `.env\0.hypercube/\0`. The actual copy command produced both:

```text
.hypercube/new/.hypercube/existing/private-draft
.hypercube/new/.hypercube/new/.git
```

The second file retained the new worktree's real Git metadata pointer. New
sessions can therefore inherit other sessions' uncommitted files and nested
worktree metadata; subsequent copies can include those earlier copies too.
APFS cloning reduces initial copying cost but does not correct the contents.

**Direction:** exclude `.hypercube` and its descendants from inclusion copying.
Keep the user's ordinary ignored-file inclusion behavior. One regression at
the existing `AddWorktree` boundary should combine a broad inclusion pattern
with an existing managed worktree. No new copy abstraction is needed.

### 3. Give overlapping Codex input calls a coherent lifetime

**High confidence; deterministic native-protocol stand-in reproduction passed
normally and under `-race`. Parent rerun confirmed it.**

The shared serve loop intentionally accepts answers while ordinary input is
awaiting native acceptance (`plugins/internal/plugin/serve.go:109`). Codex's
async-question answer calls `inputThread` too
(`plugins/codex/requests.go:446`). Two calls can consequently overlap on the
main thread, but each sets one shared `inputPending` boolean and each
completion clears it, together with `interruptInput`.

The probe held the first `turn/steer` response, completed the async answer's
steer, then interrupted. Although the first input was still outstanding,
`inputPending` was false. Interrupt targeted only the old turn. The held steer
then failed after that turn ended, and the existing late-steer fallback
started a new turn that kept running. Recorded native calls showed
`interrupts=[old]`, followed by `starts=[new]`.

There is also a concrete readability defect in this path: `answer` acquires
the mutex, while `answerAsync` releases and reacquires that caller-owned lock.
Understanding one answer requires tracking lock ownership and shared input
state across both functions.

**Direction:** accurately own outstanding main-thread input calls and retain
the interrupt request through their completion. Keep lock acquisition and
release in the same function when separating answer lookup/completion from
the blocking native call. Preserve late-steer fallback and interrupt during
start. Globally preventing answers during pending input would break the
native extension behavior that earned the shared serve-loop exception.

### 4. An obsolete request must not end the desktop alert watcher

**High confidence; deterministic interleaving reproduced normally and under
`-race`. Parent rerun confirmed it.**

`alertWatch.read` first calls `Runtime.Requests`, which returns requests from
one runtime/database snapshot. It then opens a separate database read and
looks up each request's session metadata (`internal/session/alerts.go:73`,
`:80`). A session can be deleted between those snapshots.

For a captured open offer, deleting its session makes the later lookup return
`store.ErrNotFound`. That error propagates out of `WatchAlerts`. The startup
goroutine in `cmd/hypercube/main.go:109` logs it and exits permanently. Request
and reply notifications for every other session stop until backend restart.

The reproduction used a temporary source overlay to insert a scheduling
barrier immediately after `Requests` returns. It created an offer, paused the
watcher, deleted that session, and resumed. The expected fatal stale-session
error occurred. The production code otherwise remained unchanged.

**Direction:** enrich requests within a coherent snapshot, or skip the expected
missing session when processing an older request snapshot. Preserve reporting
of genuine database failures. Adding a watcher-restart supervisor would obscure
this ownership problem.

### 5. Own file refreshes across opening, polling, and attachment changes

**High confidence; actual `openFileTab` and `createFilePoller` callers reproduced
the stale overwrite. Parent rerun confirmed it.**

Opening/selecting a file starts a read (`internal/ui/web/editor/editor.ts:97`).
Independently, polling reads active files. The poller's `running` flag only
serializes polls. The read completion checks the session ID and open path,
with no ownership check for the particular outstanding read.

In the probe, opening `a.go` started one request; polling started a second.
Resolving the second with `new text` displayed that text. Resolving the first
with `old text` then rolled the displayed content back. Ordinary agent edits
and tab clicks can reach these paths. A subsequent poll may repair the view,
but the current source display can be stale in the meantime.

Diff reads have the same ownership split (`internal/ui/web/changes/changes.ts:60`).
Session-ID guards also accept responses from an earlier attachment after an
A → B → A switch. That extension follows from the code; the retained probe
specifically demonstrates the simpler same-file overlap.

Initial and periodic tree refreshes duplicate policy too:
`files-panel.tsx:48` requests every remembered expanded folder, while
`file-polling.ts:26` requests only folders whose ancestors are expanded.
The initial path has a legitimate scroll-restoration purpose, but both paths
should use the same refresh ownership and folder-selection logic.

**Direction:** give attachment/resource reads one owner in the existing file
loading concern. Repeated callers can share current work or discard obsolete
completions. Consolidate the initial/ongoing refresh policy while preserving
scroll restoration. A held-response regression protects the real behavior;
a general data-fetching framework is unnecessary.

### 6. Encode plist strings before replacing the working service

**High confidence; isolated script-prefix execution produced invalid XML.**

The installer interpolates `APP`, `SHELL`, `SERVICE`, and the log path directly
into an XML heredoc. For example,
`APP='/Applications/Research & Development/Hypercube.app'` writes a literal
ampersand into the executable path at `scripts/install-service:42`.

Parsing the generated plist failed with
`ExpatError: not well-formed (invalid token): line 19, column 34`. The worker
executed only the prefix that writes the plist, with isolated paths; no
`launchctl` command ran. The ordinary desktop setup button passes the bundle
path to this script, so this is also a supported setup path.

The full script unloads the existing service at line 62 before bootstrap
consumes the malformed replacement. A valid macOS path can therefore break
installation and leave the prior backend unloaded.

**Direction:** write dynamic values using a plist encoder or platform plist
tool, and validate the result before `bootout`. Extend the existing isolated
installer coverage with ampersand paths. This is encoding at an external
boundary, with no need for a new installer subsystem.

### 7. Make compact transcript reads compact at the storage boundary

**High confidence in the code path; practical latency has not been benchmarked.**

`read_session` returns approximately 40,000 characters per page, but first
calls `Runtime.Transcript`. That function holds the global runtime mutex while
loading and JSON-decoding every full saved item, then merges pending items
(`internal/session/transcript.go:60`). Only afterward does MCP apply its
cursor, omit reasoning/native-child entries, reduce tools to names, and stop
at the page budget. Earlier pages repeat the full-history load.

Tool outputs and image data are copied/decoded despite being absent from the
compact response. Thus cost grows with total history and payload size rather
than the returned page. During that work, unrelated sessions also wait on the
same runtime mutex. This is a leaky boundary between a compact inspection API
and the browser's whole-transcript operation.

**Direction:** provide bounded reverse reads for this concrete consumer,
projecting away unused payloads near storage and merging pending replacements
with correct sequence/cursor handling. Snapshot pending state briefly rather
than holding the global lock through all decoding. Keep whole-transcript reads
for consumers that need them. No universal query framework, second transcript
index, or background cache is justified here.

### 8. Show directory-listing failures at the failed directory

**High confidence; actual component probe and parent rerun confirmed it.**

`loadFolder` catches all failures but handles only HTTP 404. Permission errors,
other server refusals, and fetch failures disappear. `FilesPanel` finishes its
loading phase and treats an absent listing as empty; a previously loaded
folder silently retains old entries.

With a 403 `permission denied` response for the root listing, the actual
component rendered no text, left listings as `{}`, and recorded no page error.
The backend has real file-open/ReadDir failure paths, so users can encounter
this without violating an internal invariant.

**Direction:** represent a folder's read failure and show it at that root or
folder, preserving old entries where appropriate. Existing polling already
provides a subsequent retry. Avoid producing a new global alert every second
or adding a retry/backoff subsystem. One denied-root component regression is
enough to protect the missing behavior.

### 9. Remove the parallel, inaccurate product description

**High confidence from repository contents.**

`web/content/features.md` advertises provider routing/sampling, configured
language servers, Hypercube plan-mode restrictions, and credentials managed
with `ox login`. These are not Hypercube configuration or CLI capabilities;
native harnesses own their tools, credentials, and execution modes. The claim
that file access stays inside the workspace also contradicts the deliberately
supported absolute-file opening in `internal/ui/files.go:81`.

The homepage's `curl .../install | bash` is a second installation description
without a corresponding endpoint/artifact in this repository. The maintained
installation guide is already mounted into the website. This is a repository
consistency observation; the remote endpoint was not checked.

Developer reference has related drift: `agents/architecture.md:38` says model
catalogs are never cached, while `internal/session/selection.go` persists them
and the UI normally uses `CachedModels`.

**Direction:** delete or correct the obsolete feature inventory, link the
homepage to the existing installation guide, and correct the cache statement.
Keep product facts in their existing authoritative places. Implementing the
advertised features would materially expand scope.

### 10. Avoid repeating unchanged transcript work during streaming

**High confidence in repeated work; measured cost supports P3, with no
demonstrated browser freeze.**

Every transcript delta clones the item map and sorts all keys, including when
only an existing item's text changed. `transcriptRows` then constructs new
wrapper objects for all historical rows. Those wrappers defeat the existing
shallow row memoization during streaming. Visible collapsed tool rows also
serialize full input before checking whether details are expanded
(`internal/ui/web/transcript/tool-items.tsx:20`).

A synthetic probe applied 60 one-item replacements to 10,000 tool entries and
timed update plus row projection. Parent rerun measured about 1.42 ms median
and 2.32 ms p95, excluding React/browser rendering. All row identities changed;
9,999 underlying item objects stayed identical. Timings vary between runs.

Existing optimizations matter: row projection is memoized for ordinary
composer edits; text/error leaves are memoized separately; completed activities
are normally collapsed; the server batches changes over 50 ms. These findings
do not support a claim that every keystroke reparses all Markdown.

**Direction:** retain order when existing IDs keep their sequence, and format
tool details only when opened. Preserve existing leaf memoization. Use browser
profiling before introducing stable projected-row caching or incremental turn
indexes. A replacement store framework has not earned its cost.

## Smaller concrete cleanup opportunities

These rank below the findings above and do not justify independent redesigns.

- **Build once per desktop development rebuild.** `scripts/dev:138` invokes
  `make ui`, then its desktop branch invokes `make bundle`; bundle depends on
  the phony `ui` target (`Makefile:20`). Browser installation/build runs twice.
  Let each branch invoke its complete build target once.
- **Consolidate shared file-streaming mechanics.** `internal/ui/images.go:13`
  and `internal/ui/previews.go:49` repeat open/stat/regular-file validation,
  response headers, and `ServeContent`. A small shared function can serve
  these two real callers. Keep image-type enforcement, preview path handling,
  and the PDF sandbox exception explicit; they are different policies.
- **Remove unused runtime scaffolding.** `write.runtime` in
  `internal/session/feed.go:62` is assigned but never read. Start commands
  allocate a waiter through `commands.go:53`, although start waits through
  `started`/`done` in `launch.go:105`. Removing these eliminates a dead owner
  pointer and an unused per-launch completion channel. The obsolete saved
  `last_active` column needs migration judgment and should not be bundled
  into this small cleanup automatically.
- **Put preferences under their named concern when that code next changes.**
  `internal/harness/config.go` owns theme, terminal font, notifications, file
  persistence, and configuration migration alongside harness defaults. Its
  callers are the app, UI, and runtime; native plugins need no UI preferences.
  Moving the existing configuration module to its own concern would improve
  discoverability. There is no demonstrated failure warranting a larger
  protocol/package split.

## Boundaries and complexity worth preserving

- **Holder, plugin process, and runtime have different owners and lifetimes.**
  The holder preserves native processes and output across backend replacement;
  the plugin translates a native protocol; the runtime owns saved effects and
  user/team actions. Similar short stderr tails do not establish duplicate
  process ownership.
- **Commands, queued messages, retained output, and SQLite are not redundant
  copies of one queue.** Commands preserve exact interactive requests and
  acceptance; messages await delivery; retained bytes bridge reattachment;
  committed effects/checkpoints prevent replay from applying them twice.
  Restart tests exercise these actual responsibilities.
- **The four native protocols differ materially.** Claude control requests,
  Codex turn/steer RPC, OpenCode ACP prompt/queue behavior, and Pi extension UI
  have different acceptance, question, and completion semantics. Shared serve,
  process, RPC, form, and open-request code already captures real common parts.
  A generic adapter or common answer policy would spread native exceptions.
- **Current attachment and inactive view state serve different needs.** The
  page retains view choices while refreshing server data. Draft identity
  checks protect actual in-flight sends/image reads. Removing these distinctions
  would discard behavior the project currently supports.
- **The small store and model-picker coordination are currently earned.**
  No broad unused frontend subsystem was identified. Several picker effects
  handle distinct cached-read, discovery, and effort-selection lifetimes.
  Shared context fanout alone does not justify a new state library.
- **Worktree safety rechecks are purposeful.** `RemoveWorktree` checks
  removability again after writers stop, and tests cover late writes. The
  branch and checked-out-commit checks protect different states.

Two investigated leads remain outside the ranked changes:

- Idle holder output readers poll every 25 ms, approximately 40 filesystem
  scan/open/lock-check rounds per second per reader. The code establishes
  repeated idle work; battery/CPU impact was not measured. Profile an ordinary
  multi-agent workload before paying for notification lifecycle machinery or
  another streaming transport.
- A generic `hold.Start` fixture with a detached descendant retaining stderr
  kept the holder waiting for `stderrDone` after its direct child exited.
  Shipped native wrappers already bound inherited stderr through
  `Process.WaitDelay`; terminal mode has no separate stderr reader. An ordinary
  shipped-harness trigger was not established, so this does not justify adding
  shutdown machinery to the current product.

## Coverage and verification

The review covered first-party production source across `cmd`, all `internal`
packages, all native plugins and shared plugin code, Electron, frontend state
and components, build/release/development scripts, screenshot tooling, website
source, and CI. Workers examined relevant callers and behavior tests; this is
not a claim that every test assertion or third-party file received a line-by-line
audit. Generated bundles, installed dependencies, and upstream shadcn component
style were excluded. Existing security-review conclusions were checked for
context and were not rerun as a new security audit.

Verification performed:

- Workspace-removal and stale-alert probes passed normally and together under
  `-race`. Workspace removal used the existing Git pause fixture; the alert
  probe used a temporary scheduling barrier at the identified snapshot gap.
- The Codex overlap probe passed normally and under `-race`. The parent reran
  it and both runtime probes independently.
- The parent reran the retained frontend probe: actual opening/polling callers,
  denied-root component behavior, and transcript timing/object identity checks.
- Isolated Git/copy and installer-prefix/plist parsing reproduced findings 2
  and 6. No installed service or user workspace was exercised or stopped.
- Code/caller inspection establishes finding 7's full-history load and lock
  scope; no end-to-end latency measurement is claimed.

Demonstration probes assert the bad states described here; passing them does
not mean these bugs are fixed. Temporary Go overlays left checkout source
untouched. The normal Go cache initially hit sandbox restrictions; reruns used
approved cache access or a writable temporary cache. No full `make check`, app
build, live smoke test, or visual check was run for this documentation-only
review. The only repository change is this review; nothing was committed.

Worker notes and frontend/Codex/holder probes are retained in
`/var/folders/x3/s0dy9w311sncwf87160crpw80000gn/T/hypercube-review.Eondk9q7CT/`.
Runtime overlays and probes are in `/private/tmp/hypercube-runtime-review/`.
The findings above include their evidence and limits so they remain useful
after temporary artifacts are removed.
