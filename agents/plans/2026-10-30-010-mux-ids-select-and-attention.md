# Mux IDs, select, server start, and pane attention

## Goal

Mux Milestone 2, piece 2 in `agents/mux-design.md`. Four changes in `mux/`,
so Hypercube can switch to the mux in piece 4:

- IDs carry a random part chosen when the server starts, so an ID saved from
  an earlier server names nothing and returns an "unknown" error.
- A `select {window | pane}` request, and `mux select <id>`.
- Only `mux spawn` and a bare `mux attach` start a server. Every other command
  only dials and fails with "no server is running", leaving no server behind.
  The client package offers the same two ways to connect.
- `attention` takes only a pane. A window or session shows the bell while any
  of its panes is marked.

## Related code

- `mux/internal/server/server.go` — `Server`, `nextID`, `handle`, `attention`,
  `marks` (the lookup), `respawn` (copies a pane's marks), and the `marks`
  struct that sessions, windows, and panes embed.
- `mux/internal/server/label.go` — `bell`, `sessionLabels`, and
  `windowLabels` read `attention` from sessions and windows.
- `mux/internal/server/attach.go` — the pane title reads `attention`
  (`bell(panes[i].attention)`); `layoutSession` resizes and redraws a session.
- `mux/internal/server/mouse.go` — a click on a stacked pane's title opens it
  and focuses it; `select` of a pane follows the same rule.
- `mux/client/client.go` — `Dial` and `SocketPath`.
- `mux/cmd/mux/main.go` — `run`, `call` (routes an ID by its first letter),
  and `connect`, which starts a server for every command today.
- `mux/cmd/mux/attach.go` — `attach` and `firstSession`.
- `mux/cmd/mux/main_test.go` — socket tests, `runMux`, `TestAttention`,
  `TestBellAndTitle`, `TestErrors`, `TestKillCommandStopsServer`,
  `TestSpawnCommandStartsServer`.

## Decisions

- **ID format.** `<kind><counter>-<tag>`, for example `s1-4f2a`, `w3-4f2a`,
  `p7-4f2a`. The tag is four lowercase hex digits from `math/rand/v2`, chosen
  once in `New` and shared by every ID that server gives out. The kind letter
  stays first, so `call` in the command line still routes by it. Two servers
  drawing the same tag and counter is a 1 in 65,536 chance; leave a comment
  naming it and add no code.
- **`ErrNoServer`.** `client.Dial` returns an error wrapping
  `client.ErrNoServer` ("no server is running") when the socket file is
  missing (`ENOENT`) or nothing listens on it (`ECONNREFUSED`, a socket left
  by a server that died). Other dial errors pass through. Hypercube can then
  treat "no server" as "nothing running" with `errors.Is`.
- **`client.DialOrStart(mux string)`.** Moves `connect` from `cmd/mux` into
  the client package: dial `SocketPath()`, and on `ErrNoServer` run
  `<mux> server` in its own session and dial until it answers (2 seconds, as
  today). It takes the mux executable because Hypercube's own executable is
  not the mux. The server finds the same socket path from the environment it
  inherits, so the function takes no path.
- **Which commands start a server.** `spawn` and `attach` with no session use
  `DialOrStart`; `attach <session>`, `ls`, `kill`, `label`, `attention`,
  `collapse`, and `select` use `Dial`. With no server, they print
  `mux: no server is running` and exit 1.
- **`select`.** Needs exactly one of `window` or `pane`; a session, both, or
  neither is an error ("select needs exactly one of window or pane"). A window
  becomes its session's `active` window. A pane becomes its window's `active`
  pane and that window its session's `active` window; a collapsed pane opens
  first, as a click on its title does, since a focused pane that is not drawn
  would take keys the user cannot see. `select` does not move an attached
  client to another session. It ends with `s.layoutSession(sess)` so the
  attached client redraws and an opened pane gets its size. A spawned window
  still does not become current; the caller selects it.
- **Attention lives on the pane.** Move `attention bool` from the `marks`
  struct to `pane`, so sessions and windows cannot hold one. Add
  `(*window).attention()` (any pane marked) and `(*session).attention()` (any
  window marked) for the status bar. `marks` keeps only `spans`; `respawn`
  copies `attention` along with `marks`.

## Test plan

All in `mux/cmd/mux/main_test.go`.

- `TestSpawnCommandStartsServer`: the printed IDs match
  `^s\d+-[0-9a-f]{4} w\d+-[0-9a-f]{4} p\d+-[0-9a-f]{4}\n$`.
- `TestKillCommandStopsServer`: after the server exits, `mux spawn` starts a
  new one, and `mux kill` of the first server's pane ID fails with "unknown".
  Replaces the `s1 w1 p1` check.
- `TestNoServer` (new): with a fresh state directory, `mux ls`, `mux kill`,
  `mux label`, `mux attention`, `mux collapse`, `mux select`, and
  `mux attach s1-0000` each exit 1 with "no server is running", and the socket
  file still does not exist. Add a `runMuxFails` helper that returns the
  combined output and fails the test if the command succeeded.
- `TestSelect` (new): one session with window `one` (main pane plus a stacked
  pane titled `two`, collapsed) and window `three`; attach. `select` window
  `three` makes the status bar show `1 three` reversed. `mux select` of the
  collapsed pane `two` makes window `one` current again, opens `two`, and
  shows its title reversed. Input sent next lands in `two` (checked with
  `capture`).
- `TestAttention`: a window with two panes. Marking one pane shows the bell on
  that pane's title, the window label, and the session label, and not on the
  other pane. Marking the second and clearing the first keeps the window and
  session bells. Clearing the second removes all bells.
- `TestBellAndTitle`: marks the pane instead of the window.
- `TestErrors`: add `attention` with a window, `attention` with a session,
  `select` with a session, `select` with no target, `select` with a window and
  a pane, and `select` of an unknown pane.

## Implementation plan

1. `mux/internal/server/server.go`: add `tag string` to `Server`, set in `New`;
   `nextID` formats `%c%d-%s`.
2. `mux/internal/server/server.go`, `label.go`, `attach.go`: move `attention`
   to `pane`; add the `window` and `session` `attention()` methods and use them
   in `sessionLabels` and `windowLabels`; `attention` looks up only
   `req.Pane` and errors on a window or session target ("attention needs a
   pane"); `respawn` copies `attention`.
3. `mux/internal/server/server.go`: add `select` to `handle` and a
   `(*Server).selectTarget` method (`select` is a Go keyword) as decided above.
4. `mux/client/client.go`: add `ErrNoServer`, wrap it in `Dial`, and add
   `DialOrStart`.
5. `mux/cmd/mux/main.go` and `attach.go`: delete `connect`; `spawn` and a bare
   `attach` call `client.DialOrStart` with `os.Executable()`; everything else
   calls `client.Dial(client.SocketPath())`. Add `mux select <id>` through
   `call`. Change the `attention` usage text to `<pane>` and its comment, and
   add `select` to the top usage line.
6. `mux/cmd/mux/main_test.go`: the tests above.

## Documentation updates

- `agents/mux-design.md`, "Command line" table: add
  `mux select <id>` → `select`.
