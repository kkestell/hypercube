# Security review

Five findings are fixed. The access-key proposal was withdrawn after reviewing
its cost against the documented single-user threat model.

Scope: first-party HTTP/MCP handlers, Electron, document and transcript
rendering, plugins, process and worktree handling, saved state, and build and
installation entry points. Agents running under the user's account are
trusted; websites and documents are untrusted. Isolation between different
local OS accounts is not an established product requirement.

| ID | Severity | Status | Finding | Location |
| -- | -------- | ------ | ------- | -------- |
| F1 | High | Fixed | Local HTML could read other local files | `desktop/address.js:2` |
| F2 | Medium | Fixed | Foreign pages could frame app controls | `internal/ui/ui.go:66` |
| F3 | Medium | Fixed | Vulnerable Go HTTP Range parser | `go.mod:3` |
| F4 | Medium | Fixed | State permissions exposed saved conversations | `internal/store/store.go:157` |
| F6 | Medium | Fixed | Rejected terminal GET starts a shell | `internal/ui/terminal.go:16` |

The F1–F4 fixes are committed in `509bac8`. F6 is the additional terminal fix.
F5 was withdrawn; the recorded native-client behavior remains a design note
below rather than a vulnerability under the current threat model.

## Additional finding

### F6. Rejected terminal GET starts a persistent shell

**Medium** — `internal/ui/terminal.go:16`, `internal/ui/ui.go:73`.

The handler calls `Runtime.Shell` before accepting the WebSocket handshake.
A rejected request can therefore start a panel shell and run its startup
configuration.

An ordinary GET without Origin or Upgrade headers returned 426 while the
fixture shell created its startup marker. A live Electron 45.2.3 page also
triggered this through an image request: Chromium sent a cross-site GET with
no Origin, and the shell started. An explicit foreign-Origin control returned
403 and started nothing. A cross-origin POST control left the database title
unchanged.

**Limit:** The page needs a known session ID. Discovery from a foreign website
was not demonstrated. This starts a shell; it does not establish arbitrary
command injection from a website.

**Fix:** Accept the WebSocket before starting the shell. Failed startup sends
an existing terminal-exit message and closes with status 1011 and a bounded
reason, preserving the page's behavior of closing the panel instead of retrying.
Removed the stale comment about starting a shell before socket acceptance.

The new regression failed before the fix because the rejected GET created a
holder. It passes with the race detector after the fix, including a valid
connection receiving real shell output and a long unknown session ID receiving
an exit frame and proper close status. A separate reviewer reproduced and
caught the initial unbounded close reason; the fixed reason addresses that
protocol error without adding truncation machinery.

The live Electron follow-up recorded `holderCreated:false, startupRan:false`
for the foreign image request, then `markerReturned:true, holderCreated:true,
startupRan:true` for a valid same-origin WebSocket. The negative request had no
Origin header, matching the original reproduction.

## Fixed findings

### F1. Agent browser accepted privileged local-file pages

**High** — `desktop/address.js:2`, `desktop/browser.js:38`.

Untrusted HTML opened as a `file:` page could read other files accessible to
the user. A live Electron fixture reproduced a script reading a separate
local canary. Electron documents these privileges in its
[security guidance](https://www.electronjs.org/docs/latest/tutorial/security#18-avoid-usage-of-the-file-protocol-and-prefer-usage-of-custom-protocols).

**Fix:** Restrict tool requests, toolbar entries, page navigation, redirects,
and popup targets to HTTP, HTTPS, and `about:blank`. The user approved this
restriction. Live checks refused file, data, JavaScript, and `about:srcdoc`
tool requests; HTTP remained available.

### F2. Foreign pages could frame app controls

**Medium** — `internal/ui/ui.go:66`.

A foreign page could frame the app and mislead the user into operating its
controls. Framed actions use the app's own origin, so Origin checks alone did
not prevent this. A live baseline allowed a framed session rename.

**Fix:** Send `X-Frame-Options: SAMEORIGIN`. The patched response blocked
foreign framing while retaining same-origin previews.

### F3. Build used Go with vulnerable HTTP Range parsing

**Medium** — `go.mod:3`.

Go 1.27.1's Range parser could consume excessive CPU through `ServeContent`
and static-file responses. `govulncheck` identified affected application
calls. [GO-2026-6609](https://pkg.go.dev/vuln/GO-2026-6609).

**Fix:** Require Go 1.27.3. CI reads that version from `go.mod`. The fresh Go
vulnerability scan reports no vulnerabilities.

### F4. Saved conversations had overly broad permissions

**Medium** — `internal/store/store.go:157`.

The state directory was created as `0755` and SQLite files as `0644` under
umask `022`. Other accounts could read conversations and queued commands
when parent directories permitted traversal, including supported
`XDG_STATE_HOME` locations. Reopening did not tighten permissions.

**Fix:** Create the state directory as `0700` and apply `0700` before opening
existing databases. This protects the database, sidecars, plugin state, and
logs without changing parent directories. Fresh experiments confirmed the
permission change; regression checks cover reopening and saved-row retention.

## Withdrawn F5: local-client authentication

The backend accepts native clients without an access credential. Same-UID
probes demonstrated session discovery, private fixture reads, mutations,
terminal commands, and desktop connection replacement. These exercised powers
that the documented trusted local processes already have. Different-UID
execution was not demonstrated. Loopback TCP does not itself isolate accounts,
but shared-Mac isolation was not established as a product requirement.

The user initially approved an access key, then challenged its threat model
and implementation cost. Independent Claude review recommended withdrawing
it and fixing F6 directly. The proposed key rotation, browser authentication,
and reconnect machinery was never integrated; its isolated worktree and
branch were removed. No authentication layer is part of the final change.
The existing file-permission hardening remains; it does not claim OS-account
isolation for the local API.

## Dependency advisories

Bun audits exit 1 for three moderate build/packaging dependency advisories.
No application exposure was confirmed after checking their prerequisites and
actual call paths.

| Advisory | Dependency | Assessment |
| -------- | ---------- | ---------- |
| [GHSA-rj75-hqrm-r3gf](https://github.com/postcss/postcss-selector-parser/security/advisories/GHSA-rj75-hqrm-r3gf) | `postcss-selector-parser@6.0.10` | Typography supplies fixed bundled selectors; runtime documents and filenames do not reach this parser. Same-major fix: 6.1.5; typography pins 6.0.10. |
| [GHSA-3g3m-2fjr-6pw4](https://github.com/isaacs/node-glob/security/advisories/GHSA-3g3m-2fjr-6pw4) | `glob@13.0.6` through asar | Packaging uses `--no-asar`, disabling the relevant crawl. The ordinary asar call also lacks the AbortSignal required for this failure. Fix: 14.0.2. |
| [GHSA-g3hv-75wg-m249](https://github.com/isaacs/minimatch/security/advisories/GHSA-g3hv-75wg-m249) | `minimatch@10.2.6` through asar/glob | Archive paths are disabled. The hypothetical compiled pattern is a trusted staging path plus a fixed suffix, not an untrusted filename. Fix: 11.0.1. |

## Verification and limits

Independent experiments used real production packages, isolated SQLite state,
marker files, controlled native processes, and a separate Electron profile:

- **31 plugin experiments passed:** literal input transport, approval and
  denial replies, permission scopes, cancellation, unsupported requests,
  malformed output, and unchanged `HOME`. All four plugin processes exited
  cleanly.
- **41 file/process/worktree observations:** relative traversal and outward
  symlinks were refused; search stayed within its root; arguments and Git
  paths stayed literal; configured external diff commands did not run;
  invalid child names were refused; nonempty worktree destinations retained
  their contents. Positive controls succeeded.
- **Live rendering:** 15 Markdown cases, five highlighting cases, and image
  allowlist checks produced no payload execution. The HTML execution control
  succeeded. HTML and SVG previews blocked scripts; HTML labeled PDF did not
  execute as HTML. Agent browser pages exposed no Node or app/preload bridge.
- **Dependency scans:** `govulncheck` passed; Bun reported the three advisories
  above. Upstream advisory conditions and patched versions were verified.
- **Fix validation:** The F6 regression failed before the fix, then passed
  under the race detector. Its live Electron follow-up passed with a valid
  WebSocket as the positive control. `make check` passed on clean `b16b6ee`
  plus this exact terminal patch: generated assets, TypeScript, 253 UI tests,
  27 desktop tests, vet, formatting, and all Go race tests. The shared checkout
  check stopped at stale generated assets during concurrent UI work; those
  edits were left alone. UI tests emitted an existing React `act` warning.

Evidence, reproduction scripts, and the tested source snapshot are in
`/tmp/hc-audit-lmdicsv4/`; the earlier reviewer probes remain in
`/tmp/hc-fs-review-_t7jw1u0` and `/tmp/hypercube-plugin-probe.jGmzbm`. The
follow-up snapshot is `b16b6ee14285aa67524152815ed326169315b525`; its SHA-256
manifest is `reviewed-source-hashes.json` in that directory. All fixture
processes ended. The installed app was left alone.

No additional distinct first-party security finding was confirmed. Native
providers, browser/PDF engines, all third-party source, launchd behavior, and
every browser navigation variant were not independently audited. The review
does not establish security beyond the boundaries and experiments described.
