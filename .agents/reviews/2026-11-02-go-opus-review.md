# Go review of 760eeae..f55f444

Reviewer: go-opus-review (Opus 5.5). Review only; no code changed.

## Conclusion

I found no confirmed correctness bug in production code paths. A focused
`go test -race -count=1 ./internal/session/ ./internal/hold/ ./internal/harness/`
passed. Three findings block on readability, because they concern lifecycle
ownership, lock ownership, and the holder's on-disk contract. The rest are
deletions and text corrections. Findings are ranked by severity.

## Findings

### 1. Blocking: CreateChild closes a launch's `done` channel itself

`internal/session/children.go:87-110`

- **Violation.** Each lifecycle needs one owner. `run` closes `l.done` in its
  defer (`launch.go:119`), and every other caller waits on that close. When
  `workspace.AddWorktree` fails, CreateChild instead calls `r.end(l, err)` and
  then `close(l.done)` (`children.go:91-92`). A `created` flag tracks a third
  state.
- **Evidence.** On a worktree failure:
  1. `endLocked` (`launch.go:239`) writes "Could not start the agent: …" into
     the child's transcript, saves the session, deletes its commands, and
     removes it from `r.live`.
  2. `end` calls `os.RemoveAll` on a holder directory that was never created.
  3. CreateChild closes `done` by hand.
  4. Fifteen lines later, `DeleteSession(child.ID)` throws all of that away.

  To see why the manual close is safe, a reader has to know that `run` never
  started. They also need to know that `Detach` and `stopLaunches` may already
  be blocked on `<-l.done` during the slow `git worktree add`.
- **Fix.** Create the worktree before the launch exists:
  - Move the duplicate-name and `NoGit` checks, and the `Dir`/`Branch`
    computation, into the first `db.Read` (`children.go:27`).
  - Call `workspace.AddWorktree` before `r.mu.Lock()`.
  - On a later failure, remove the worktree and the row as the existing
    cleanup already does.
  - Delete lines 87-95 (`created`, `r.end`, `close(l.done)`), and change
    `if created` to `if child.Branch != ""`.

  The `UNIQUE (parent, name)` constraint and the branch's existence still
  reject a concurrent duplicate. Expected size is about −10/+10 lines, with one
  fewer owner of `done`.

### 2. Blocking: `live` documents neither its fields nor which lock guards them

`internal/session/launch.go:19-36`, `internal/session/runtime.go:38-41`

- **Violation.** The Runtime comment says `mu` guards "the launches in live,
  their fields". That is wrong for two fields:
  - `sent` is read and written with only `l.send` held (`commands.go:44-68`).
  - `Detach` reads `client` while holding only `l.send`
    (`sessions.go:414-420`). This is safe only because `run` assigns `client`
    under `mu` before `Detach` sets `detached` under `mu`. A reader must
    reconstruct that ordering.

  None of the 16 fields has a comment. Their meanings have to be pieced
  together from six files:
  - `ready`: Unix milliseconds of the start reply, or zero while starting.
  - `position`: the output offset applied so far.
  - `quick`: the consecutive quick-exit count.
  - `cause`: why the launch ended.
  - `prefix`: the launch ID plus ":", which namespaces item IDs.
  - `attention`: the last saved attention flag.
- **Fix.** Correct the Runtime comment to say that `mu` guards every field
  except `sent`. Then group `live`'s fields:
  - fields guarded by `mu`;
  - `send`, which serializes writes to `client` and guards `sent`;
  - `done`, which `run` closes.

  Add one line to each non-obvious field. This is about +10 comment lines and
  no code.

### 3. Blocking: hold's on-disk contract is undocumented

`internal/hold/hold.go:1-3, 30-223`, `internal/hold/holder.go:63-81, 159-179`,
`internal/hold/output.go`

- **Violation.** Restore and the output reader depend on several file-level
  guarantees:
  - The holder holds `lock` for its whole life; `Alive` tests this.
  - `exit` is renamed into place before the lock is released
    (`restore.go:84-86`, `output.go:68-70`).
  - The holder also keeps `pid`, `tty`, and the `in` socket in its directory.
  - Output segments are named `out-<byte offset>`. Terminal mode keeps the last
    two segments; pipe mode keeps every segment until `Discard`.

  The package comment says only "a directory for input, retained output, and
  the child's exit status." `Holder`, `Size`, `Exit`, and the methods `Open`,
  `Write`, `Alive`, `Stop`, `Exit`, `Remove`, `Resize`, and `Discard` have no
  doc comments. The 1-second output drain (`holder.go:66-71`) and the 3-second
  grace before SIGKILL (`holder.go:174`) have no comment explaining why.
- **Fix.** Write a package comment that lists the files and the ordering
  guarantee (about 8 lines). Add one line per exported identifier, and a
  why-comment on each timeout. This changes documentation only.

### 4. Deletion: `Holder.First` is redundant

`internal/hold/hold.go:203-209`, `internal/terminal/terminal.go:130`

- **Evidence.** `Output.Read` already moves a position before the first
  segment up to that segment (`output.go:38-40`). So `Output(0)` behaves the
  same as `Output(First())`. `First` has no other caller.
- **Fix.** Change `terminal.go:130` to `return sh.holder.Output(0)` and delete
  `First`. This removes 7 lines.

### 5. Deletion: guards repeated in one call path

`internal/session/event.go`, `internal/session/commands.go`,
`internal/session/input.go`

- **`commands.go:102-105`.** This repeats the unknown-reply check that `apply`
  makes just before, under the same lock (`event.go:24-26`). Keep
  `c := l.commands[reply.ID]` and delete the check. About −3 lines.
- **`event.go:72-74`.** `eventLocked` re-checks `r.current(l)`. Its only
  production caller, `apply` (`event.go:17`), checked this under the same lock,
  and the test helper (`mirrors_test.go:34`) passes a current launch. Delete
  the check: −3 lines.
- **`event.go:164-166`.** The `if l.client != nil` guard cannot fail. Events
  arrive only through `client.Read`, and `apply` calls `l.client.Discard`
  unguarded at `event.go:44`. Keep the call and drop the guard: −2 lines.
- **`input.go:78-82`.** `Answer` repeats `running()`'s condition
  (`input.go:12-17`). Use `l, err := r.running(id)` and return "the request is
  finished" on error. About −1 line.

### 6. Deletion: `harness.Spec` has one use

`internal/harness/config.go:10-22, 79-89`

- **Evidence.** Since roles were removed, `Spec` is only embedded in `Profile`.
  Tests spell profiles both ways:
  - `Profile{Spec: harness.Spec{...}}` in `children_test.go:24,39,61,73`,
    `launch_test.go:53,117`, and `report_test.go:39`;
  - `Profile{Harness: ...}` in `children_test.go:146,196`,
    `inactivity_test.go:60`, and `ui_test.go:535`.
- **Fix.** Declare `Harness`, `Model`, and `Effort` directly in `Profile`. Make
  `check` a `Profile` method, or fold it into `LoadConfig`. Rewrite the 7 test
  literals. This removes one type and about 4 lines, and leaves the JSON
  unchanged.

### 7. Inaccurate text

- **Timeout messages state fixed values.** Each message hard-codes a duration
  that comes from a named value:
  - `internal/session/launch.go:165` says "within a minute"; the bound is
    `r.startTimeout`. `restart_test.go:380` asserts that text with
    `startTimeout` set to 2s.
  - `internal/session/deliver.go:71` says "within 30 seconds"; the bound is
    `r.requestTimeout`.
  - `internal/mcp/browser.go:50,72` say "within 30 seconds"; the bound is
    `browserTimeout`.

  Fix: build the text from the value, for example
  `fmt.Errorf("the plugin did not start within %s", r.startTimeout)`, and
  update the test. No net change in size.
- **`internal/store/store.go:138`.** "Open opens the database at Path": there
  is no `Path`. It should say "in Dir".
- **`internal/session/sessions.go:390`.** "Quit ends children and shells"
  means agent processes. Now that child sessions are a concept, it reads as
  child sessions only. It should say "ends every agent and shell".
- **`internal/session/launch_test.go:52`.** "AddWorker refuses" should name
  `CreateChild`.

### 8. Tests: vestiges of the lead/worker model, and tests of things that cannot break

- **`internal/ui/ui_test.go:766-770, 803, 814`.** Every call passes "lead" as
  `seedSnapshot`'s first name, and `names[1:]` skips it. Drop that argument.
- **`cmd/hypercube/integration_test.go`.**
  - Line 200: `client.newSession`'s `parent` parameter is a profile name.
    Rename it to `profile`.
  - Lines 315-317 and 618: `w.family(id)[0]` is the session itself. Use `id`.
- **`internal/report/report_test.go:61-63, 74-79`.** These assert that removed
  wording is absent ("Other top-level sessions:", "lead", "worker"). Delete
  them: −9 lines.
- **`plugins/codex/storage_test.go:69-88`.** This exercises the operating
  system renaming a file through a symlink's target, not `prepareStorage`.
  Delete it: −20 lines.
- **`plugins/codex/main_test.go:917, 936`.** `p["threadSource"] != nil`
  asserts that a removed parameter is absent. Drop the two conditions.
- **`plugins/internal/plugin/serve_test.go:211-222`.** Once `shutdown` was
  removed, `TestServeEndsWithItsInput` became a subset of two other tests:
  `TestServeDrivesTheAgent` (lines 182-189) and `TestServeIgnoresResentCommands`.
  Both close the input and check that the agent recorded "stop". Delete it:
  −12 lines.
- **Removed role and tool names in plugin tests.** These still pass, because
  each test checks only how a name is split. Use current names:
  - `plugins/opencode/main_test.go:964` uses `hypercube_create_agent`; use
    `hypercube_create_session`.
  - `plugins/pi/main_test.go:823, 825` use `remove_agent`; use `end_session`.
  - `plugins/claude/main_test.go:724` sends `"to":"lead"`.
  - `plugins/claude/main_test.go:576, 599` use the instructions "You lead a
    team.".

## Checked and not reported

- **Escaped descendant holding stderr.** A pipe-mode child whose descendant
  calls `setsid` and keeps stderr open keeps the holder alive. `<-stderrDone`
  in `holder.go:73` has no deadline, unlike stdout. While it waits, the holder
  ignores Stop's SIGTERM.

  I confirmed this in a scratch copy outside the repo. A Perl child forked a
  `setsid` process that slept 8 seconds. Output ended only after 8 seconds, and
  the holder stayed alive 6 seconds after `Stop`. With only stdout held, output
  ended after 1 second.

  The current plugins cannot reach this: `plugin.StartProcess` gives each
  native harness its own stderr pipe (`plugins/internal/plugin/process.go:66`).
  I do not report it as a finding.
- **Data races.** The race-enabled tests listed above passed.
  - `Detach` reads `client` without `mu`, but `mu` orders that read after
    `run`'s write.
  - `wait` reads `cause` after `<-l.done`, and `run` writes it before closing
    `done`.
- **Idle polling.** Every attached reader polls its holder directory every
  25 ms (`output.go:83`). I measured no cost, so I do not report it.

## Coverage

I read every changed in-scope file in full in its current state, including
each plugin file, which I first covered by its diff alone and then read in
full. Deleted files are listed as deleted.

**cmd/**
- `cmd/hypercube/main.go`
- `cmd/hypercube/integration_test.go`
- `cmd/hypercube/testmain_test.go`
- `cmd/hypercube/main_test.go` (unchanged; read as a caller)

**internal/desktop**
- `app.m`, `browser.go`, `browser.m`, `desktop.go`, `desktop.h`, `window.m`

**internal/harness**
- `harness.go`, `protocol.go`, `config.go`
- `client_test.go`, `harness_test.go`, `testmain_test.go`
- `harnesstest/harnesstest.go`

**internal/hold**
- `hold.go`, `holder.go`, `output.go`
- `hold_test.go`, `testmain_test.go`
- `holdtest/holdtest.go`

**internal/mcp**
- `mcp.go`, `browser.go`
- `mcp_test.go`, `browser_test.go`, `testmain_test.go`

**internal/menu**
- `menu.go`, `menu.m`: deleted. No references remain.

**internal/report**
- `report.go`, `report_test.go`

**internal/session**
- `runtime.go`, `sessions.go`, `launch.go`, `restore.go`, `commands.go`,
  `deliver.go`, `input.go`, `event.go`, `feed.go`, `inactivity.go`,
  `messages.go`, `transcript.go`, `children.go`
- Tests: `fixture_test.go`, `testmain_test.go`, `children_test.go`,
  `deliver_test.go`, `event_test.go`, `feed_test.go`, `inactivity_test.go`,
  `input_test.go`, `launch_test.go`, `messages_test.go`, `mirrors_test.go`,
  `restart_test.go`, `sessions_test.go`, `transcript_test.go`
- `workers.go`, `workers_test.go`: deleted.

**internal/store**
- `store.go`, `migrate.go`
- `store_test.go`, `migrate_test.go`
- `testdata/origin-schema.sql`

**internal/terminal**
- `terminal.go`, `terminal_test.go`, `testmain_test.go`

**internal/ui (Go)**
- `ui.go`, `actions.go`, `files.go`, `images.go`, `snapshot.go`, `socket.go`,
  `terminal.go`
- Tests: `files_test.go`, `ui_test.go`, `testmain_test.go`
- `assets.go` (unchanged; read as a dependency)

**internal/workspace**
- `workspace.go`, `workspace_test.go`

**plugins/**
- `plugins/internal/plugin/serve.go`, `serve_test.go`
- `plugins/codex/main.go`, `main_test.go`, `storage.go`, `storage_test.go`
- `plugins/opencode/main.go`, `main_test.go`
- `plugins/claude/main_test.go`
- `plugins/pi/main_test.go`
- `plugins/internal/plugin/process.go` (unchanged): read only its stderr
  wiring, to check the item under "Checked and not reported".

**Outside Go scope, not reviewed**
- `internal/ui/web/**`: frontend reviewer.
- Generated build output: `internal/ui/dist/main.js`, `internal/ui/dist/main.css`,
  and `internal/ui/dist/highlight-csd6f986.js`, renamed to
  `highlight-33segak0.js`. The frontend build owns these, and `make check`
  verifies they are current.
