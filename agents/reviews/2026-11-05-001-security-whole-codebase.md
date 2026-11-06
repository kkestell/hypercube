# Repository security review

## Recovery note

Updated on 2026-11-06 by extracting findings from the previous review session,
including its later runtime experiments and reviewer responses. This is a
record of that work, not a new audit. The original report omitted F5 and F6.
The four earlier fixes remain in the checkout; F5 and F6 were not implemented
before the session stopped. Severity was not assigned to either later finding.

Source session: `01a1af42-f588-7d82-8fe2-a0bad4fe78a1`
(“Review Whole Codebase Security”). Runtime HTTP findings came from reviewer
`01a1af69-ded2-7130-9254-f03e26910b4b`. Findings below preserve the distinction
between observed behavior and inferred impact.

## Scope and coverage

Repository-wide security review of first-party production source in `cmd/`,
`internal/`, `plugins/`, `desktop/`, `scripts/`, the Makefile, and CI workflows.
After an incomplete first pass, four reviewers with fresh contexts divided
the source into the scopes below. The primary reviewer checked their results,
fixed the confirmed findings, and verified the combined changes.

| Scope | Coverage |
| ----- | -------- |
| Session and saved state | All production source and Go tests in `internal/session/`, `store/`, `report/`, `workspace/`, `hold/`, and `terminal/`; traced persistence, approvals, message delivery, worktrees, and process ownership. |
| Native harnesses and plugins | All source and tests in `internal/harness/` and `plugins/`; traced per-launch configuration, process arguments, native requests, approval and cancellation mappings, and transcript events. |
| Frontend and Electron | All production source, declarations, markup, stylesheets, SVG assets, and component wrappers in `internal/ui/web/` and `desktop/`; all desktop tests and relevant frontend boundary tests; traced rendering, URLs, IPC, storage, previews, and terminal input. |
| Backend and build entry points | All production source and Go tests in `cmd/`, `internal/ui/*.go`, `internal/mcp/`, `internal/desktop/`, and `internal/buildinfo/`; all scripts, Makefile, and workflows; traced HTTP, WebSockets, MCP identity and scope, file access, desktop calls, and packaging/install commands. |

Read supporting architecture and user documentation. Applied the documented
model: one user, trusted local agents running with the user's credentials,
and untrusted website and document content. Intended agent execution, browser
evaluation, arbitrary local file viewing, and documented approval policies
were not treated as security defects merely because they are powerful.

Coverage limits:

- Third-party dependency source, native harness internals, Chromium/PDF engine
  internals, and generated bundles were not independently audited. Generated
  UI assets were checked against a fresh build. Relevant native fixture
  records were inspected; the raw fixtures were not all read line by line.
- Dependency checks used the current Go and npm advisory databases. The
  installed application was not changed or independently audited, and native
  providers and launchd installation were not exercised.
- A live Electron 45.2.3 fixture reproduced the local-file read and checked
  the navigation fix with isolated files and a temporary profile. Later live
  experiments exercised framing, document isolation, rendering, and additional
  navigation cases, recorded below. No browser-engine fuzzing or visual checks
  were performed.

## Summary

| ID | Severity | Category | Fix | Title |
| -- | -------- | -------- | --- | ----- |
| F1 | High | Security | Fixed | Agent browser accepts privileged local-file pages |
| F2 | Medium | Security | Fixed | Foreign pages can frame the app's controls |
| F3 | Medium | Dependencies | Fixed | Build uses Go with vulnerable HTTP Range parsing |
| F4 | Medium | Security | Fixed | Saved conversations have permissions that allow other accounts to read them |
| F5 | Not assigned | Security | Open; fix approved | Backend callers are not authenticated |
| F6 | Not assigned | Security | Open; browser impact unverified | Rejected terminal GET starts a persistent shell |

## F1. Agent browser accepts privileged local-file pages

High · Security · Fixed · `desktop/address.js:2`, `desktop/browser.js:38`

Opening untrusted local HTML in the agent browser allowed its scripts to run
as a `file:` page. Electron gives these pages access to other local files,
so a document can expose files readable by the user's account. The site
view's lack of a preload bridge does not remove these file privileges.

### Evidence

Before the fix, `normalizeAddress('file:///tmp/untrusted.html')` returned the
file URL. Both the tool and toolbar passed that result to `loadURL`. Popup
links bypassed normalization entirely, and page navigation and redirects had
no URL restrictions. In a live Electron 45.2.3 fixture, the pre-fix browser
source loaded an HTML page whose script fetched a separate temporary text
file. The patched source refused that navigation and still loaded an HTTP
page and `about:blank`. Only disposable fixture files were read; no user
secrets or external collector were involved. Electron documents the
additional file access in its
[security guidance](https://www.electronjs.org/docs/latest/tutorial/security#18-avoid-usage-of-the-file-protocol-and-prefer-usage-of-custom-protocols).

### Fix

This finding initially needed a decision because restricting navigation
removes an existing capability. The user chose HTTP, HTTPS, and `about:blank`
only. Added one URL predicate shared by address normalization and navigation
guards. Tool requests, toolbar entries, page navigation, redirects, and popup
links now enforce that restriction. Added URL regression cases and documented
the supported addresses. Existing sandboxed file previews remain available.

## F2. Foreign pages can frame the app's controls

Medium · Security · Fixed · `internal/ui/ui.go:66`

A hostile page could embed Hypercube in an iframe and hide its controls
under misleading content. A user's clicks could then answer an approval or
trigger another app action. Actions initiated by the embedded app have the
app's own origin, so the existing origin check does not prevent this.

### Evidence

The main document accepted GET requests without an Origin header, as browser
frame navigations can send. Neither the response nor the page declared a
framing restriction. The embedded app would connect its data socket and send
actions with the same origin the server already accepts.

### Fix

Trivial: added `X-Frame-Options: SAMEORIGIN` at the HTTP boundary. Foreign
ancestors cannot frame the app; same-origin document viewers remain allowed.
Added a regression test for the main page's response header. Existing origin,
WebSocket, and preview tests pass.

## F3. Build uses Go with vulnerable HTTP Range parsing

Medium · Dependencies · Fixed · `go.mod:3`

The build required Go 1.27.1. A client able to reach the loopback server could
exercise its vulnerable Range parser through static assets, image responses,
or document previews, consuming excessive CPU. This finding does not assert
that a foreign website can bypass browser CORS restrictions to send that
header.

### Evidence

`govulncheck` identified calls to `http.ServeContent` and the static file
handler. Those functions are affected by
[GO-2026-6609](https://pkg.go.dev/vuln/GO-2026-6609), fixed in Go 1.27.2.
Other scan matches included features the app does not use, such as HTTP/2
serving and Windows root operations; those were not separate findings.

### Fix

Trivial: raised the minimum Go version to 1.27.3. That patch also includes
the certificate trust correction in
[GO-2026-6809](https://pkg.go.dev/vuln/GO-2026-6809). The repository's CI reads
the version from `go.mod`, so no parallel version setting was needed.
The subsequent Go vulnerability scan reported no vulnerabilities.

## F4. Saved conversations have permissions that allow other accounts to read them

Medium · Security · Fixed · `internal/store/store.go:157`

Other OS accounts could read saved prompts, tool data, conversations, queued
messages, and pending commands when the state directory's ancestors allowed
traversal. This can happen with the supported `XDG_STATE_HOME` setting; a
private home directory can otherwise mask the problem.

### Evidence

`store.Open()` created the application state directory with `0755`, and SQLite
created `hypercube.db` with `0644` under the ordinary `022` umask. An isolated
permission check reproduced both modes. Reopening left those permissions
unchanged. No separate OS account was used for the reproduction.

### Fix

Trivial: create the state directory with `0700` and apply `0700` before opening
SQLite in existing installations. Protecting the enclosing directory covers
the database, SQLite sidecars, plugin state, and logs without altering parent
directories. Extended the existing database-reopen test to check fresh and
existing directory permissions while preserving saved rows. Both permission
checks failed before the fix and passed after it. A second reviewer checked
the fix and the production paths that write or clean up state.

## F5. Backend callers are not authenticated

Security · Open; fix approved · `internal/ui/ui.go:68`,
`internal/mcp/mcp.go:190`, `internal/desktop/desktop.go:42`

Host and Origin checks do not authenticate native clients: those clients can
supply either header. The loopback backend has no credential separating the
app owner's access from another local OS account. Same-account agents are
trusted by design; the finding concerns untrusted processes under other
accounts on the same Mac.

### Evidence

Fresh experiments against the real `ui.Handler`, with isolated SQLite state
and disposable marker files, demonstrated all of the following without a
credential:

- A `/state` WebSocket without Origin disclosed seeded session IDs, titles,
  and directories.
- `/file` with a disclosed session ID and an absolute path read a private
  fixture file with mode `0600`.
- A `/session?action=rename` POST with matching Origin changed the actual
  database title.
- `/mcp`, using a disclosed ID as `X-Hypercube-Session`, accepted `set_title`
  and changed the actual database title. A known session ID identifies the
  caller but does not authenticate it.
- A `/terminal` WebSocket without Origin accepted a harmless shell command
  that created a temporary marker file.
- A second `/desktop` WebSocket without Origin displaced the original desktop
  connection, received a real `eval` request, and supplied a forged reply that
  the backend accepted as the successful evaluation result.

The experiments ran as UID 501. Switching to another account was unavailable
because `sudo -n -u nobody id` required a password. Cross-account applicability
is inferred from loopback TCP access without UID-bearing admission; an actual
cross-account execution was not demonstrated. These experiments do not show
that a foreign website can bypass the browser's Origin or preflight rules.

### Required follow-up and recovered user decision

The reviewer recommended authenticating backend callers with a secret held in
the protected state directory, covering browser, native MCP, and desktop
connections, while retaining Host and Origin checks for browser isolation.

The user answered **“Require a per-user access key (Recommended)”**. That
decision was received before the automated filter stopped further responses.
The question described authenticated browser links replacing bare-localhost
access and active sessions needing to restart. Implementation remains open;
the decision does not need to be requested again.

## F6. Rejected terminal GET starts a persistent shell

Security · Open; browser impact unverified · `internal/ui/terminal.go:25`,
`internal/ui/terminal.go:29`, `internal/ui/ui.go:74`

The terminal handler calls `Runtime.Shell` before validating the WebSocket
handshake. Ordinary GET requests without Origin pass the outer admission
check. Consequently, a request ultimately rejected as an invalid WebSocket
can still start a persistent panel shell and run its startup configuration.

### Evidence

Against a fresh fixture session, an ordinary GET to `/terminal?session=<id>`
with no Origin or Upgrade headers returned 426, but the shell holder directory
and a marker written by the controlled shell's startup command appeared.
The request supplied `Sec-Fetch-Site: cross-site`, `Sec-Fetch-Mode: no-cors`,
and `Sec-Fetch-Dest: image`. An explicit foreign-Origin control returned 403
and created neither the holder nor the marker.

The reviewer proposed that an untrusted page knowing a session ID could
potentially trigger this through an image request. Actual browser delivery
was not verified. This establishes shell-start side effects on a rejected
HTTP request, not arbitrary command injection from a website.

### Recommended fix

Accept the WebSocket successfully before starting the shell. No fix for this
finding was recorded in the prior session.

## Later runtime experiments recovered from the session

These results supplement the original checks; they were not reflected in the
original four-finding report.

- **Framing:** In a live Electron 45.2.3 fixture, the baseline without the
  framing header allowed the framed app origin to perform a session rename
  (HTTP 200). The patched response was blocked by `X-Frame-Options:
  SAMEORIGIN`. This closes the earlier live framing evidence gap for F2;
  it was not a visual demonstration of a user being tricked into clicking.
- **Document isolation:** A positive control executed a document script.
  Actual HTML, SVG, and sniffed-HTML preview responses blocked scripts through
  sandboxing. HTML bytes named as PDF did not execute the script or expose
  Node or the preload bridge. This is evidence for those fixtures, not a PDF
  engine audit.
- **Rendering:** Fifteen Markdown payload cases produced no active injected
  elements or unsafe links. Highlighting in five language cases escaped and
  preserved the supplied text. Transcript images rejected SVG and HTML MIME
  types and retained the raster case. The HTML execution control succeeded;
  payload executions remained zero.
- **Navigation:** Live tool requests refused `file:`, `data:`, `about:srcdoc`,
  and `javascript:` cases. Chromium refused top-level data navigation and an
  unsafe redirect. The final `TestSecurityRuntimeIndependent` overlay run
  passed after fixing fixture setup and serving its JavaScript as UTF-8.
  Earlier fixture runs failed; they were not additional application findings.
- **Files and processes:** Independent experiments confirmed relative-path
  and outward-symlink confinement, search confinement, state-directory
  tightening, literal subprocess arguments, child-name rejection, literal Git
  pathspecs, and suppression of configured external diff commands. Absolute
  file reads were permitted by explicit design and amplify F5. Following a
  `.hypercube` symlink and resolving a trusted harness name outside its
  directory were observations without an independent attacker boundary.
- **Plugins:** Thirty-one independent experiments against freshly built
  first-party plugin executables found no additional confirmed security
  issue. They exercised literal prompt/image transport, approval and denial
  mappings, cancellation, unsupported requests, and malformed-output recovery.
  Native processes were controlled stand-ins; installed providers and their
  settings-merging behavior were not exercised.
- **HTTP controls:** Foreign, opaque, and omitted-Origin POST attempts did not
  rename sessions; matching-Origin requests did. Foreign and opaque Origins
  were refused on state sockets. Tested rebinding Host aliases, encoded MCP
  path variants, and missing/unknown MCP identities did not bypass their
  checks. Cross-origin MCP preflight returned 405. These controls do not
  address unauthenticated native clients in F5.

Recovered reviewer evidence locations, not re-run during extraction:

- File/process reviewer `01a1af6a-7e65-7161-b459-5ca4167a9473`:
  `/tmp/hc-fs-review-_t7jw1u0` (`results.jsonl`, `native-results.json`, probe
  sources and native argument captures).
- Plugin reviewer `01a1af6a-dfba-7863-b392-85b49accf5fa`:
  `/tmp/hypercube-plugin-probe.jGmzbm` (probe sources, `results.json`, protocol
  records).
- Primary browser fixture:
  `/var/folders/x3/s0dy9w311sncwf87160crpw80000gn/T/hypercube-security-runtime-z8rcogcw`.
- The HTTP reviewer reported cleaning its owned processes and temporary probe
  files. Its finding text remains in the reviewer session identified above.

## Dependency advisory follow-up

The later dependency review found no confirmed exploitable path through
Hypercube's runtime or normal build/package workflows for these three locked
dependencies. They remain advisory matches, not additional confirmed
first-party vulnerabilities:

| Advisory | Dependency | Recovered reachability assessment |
| -------- | ---------- | ---------------------------------- |
| GHSA-rj75-hqrm-r3gf | `postcss-selector-parser@6.0.10` | Typography supplies fixed bundled selectors; runtime documents and filenames do not reach this parser. Instrumentation observed 354 calls, 93 unique selectors, maximum length 42. The same-major fix is 6.1.5, but typography pins 6.0.10. |
| GHSA-3g3m-2fjr-6pw4 | `glob@13.0.6` through Electron packager/asar | `--no-asar` disables the relevant packaging crawl. The hypothetical ordinary asar call also supplies no AbortSignal, required for the advisory's cancellation condition. The reported fix is 14.0.2, outside the declared range. |
| GHSA-g3hv-75wg-m249 | `minimatch@10.2.6` through asar/glob | Relevant archive paths are disabled and these tools are excluded from the shipped app. Exploitation requires a controlled compiled pattern with optimization level at least 2; the hypothetical glob pattern is the trusted staging path plus a fixed suffix. The reported fix is 11.0.1, outside the declared ranges. |

These assessments and version numbers are extracted from the previous session,
not newly checked advisory data. Major dependency overrides were not proposed
as confirmed security fixes.

## Checks run

- `make check`: passed after the final code changes, including generated
  asset verification, TypeScript, 238 UI tests, 27 desktop tests, vet,
  formatting, and all Go tests under the race detector. UI tests emitted an
  existing React `act` warning but had no failures.
- Focused Go origin, framing, and browser-preview tests: passed.
- `bun test address.test.js` in `desktop/`: 14 tests passed.
- One-off check of the actual `desktop/browser.js` with Electron API
  stand-ins: tool, toolbar, page-navigation, redirect, and popup restrictions
  passed; ordinary HTTPS navigation remained allowed.
- Live Electron 45.2.3 fixture using the actual pre-fix and patched browser
  modules: reproduced cross-file reading before the fix, refused local-file
  navigation after it, and retained HTTP and `about:blank` navigation. The
  fixture used a temporary app copy, profile, HTML page, and sentinel file;
  its processes and files were removed afterward.
- `go test -race ./internal/store -run '^TestOpenSecuresStateAndPreservesRows$' -count=1`:
  passed for new and existing installations. The same test failed on both
  permission assertions before the fix.
- Reviewers also ran the plugin/harness race tests, all desktop tests, 55
  selected frontend boundary tests, and focused HTTP/MCP/desktop tests; passed.
- `go run golang.org/x/vuln/cmd/govulncheck@latest ./...`: initially found
  vulnerable standard-library symbols; passed after the Go update.
- `bun audit` in `internal/ui/`: exited 1 for the moderate
  `postcss-selector-parser` advisory GHSA-rj75-hqrm-r3gf, used by the build's
  typography plugin.
- `bun audit` in `desktop/`: exited 1 for moderate packaging-tool advisories
  GHSA-3g3m-2fjr-6pw4 (`glob`) and GHSA-g3hv-75wg-m249 (`minimatch`).
  No attacker-controlled input path to these build/packaging parsers was
  confirmed. These advisory matches remain; dependency upgrades were not
  treated as confirmed application fixes.
- Tracked-source private-key and common token-pattern scan: no matches.
- `git diff --check`: passed.

## Verdict

F1–F4 were fixed in the checkout: one High and three Medium. F5 remains open
with the user's approval to require a per-user access key. F6 remains open;
its rejected-request side effect was reproduced, while its proposed browser
trigger was not verified. Build-tool advisory matches remain without a
confirmed application exploit path.

The existing four fixes require a rebuilt backend and desktop app, without
active agent session restarts. The proposed access-key change was described
as requiring active sessions to restart once implemented. The installed
application was left alone. Extraction changed only this report and ran no
application checks.
