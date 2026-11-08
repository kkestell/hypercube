# Runtime integration review

## Scope and coverage

Reviewed all 13 staged files for
[plan 011](../plans/2026-11-07-011-runtime-integration.md) and its
[work log](../work/2026-11-08-003-runtime-integration.md). Read the whole
touched Makefile, packaging rules, development script, Linux delivery tests,
screenshot driver, and developer references. Traced executable discovery,
agent and panel-shell launch, saved MCP command compatibility, holder
reattachment and cleanup, bundle installation, and release packaging.

Reused the implementation's reported passing `make check`, standalone
binaries build, macOS build, Debian ARM64 bundle/install/release tests,
dependency checks, and isolated development lifecycle checks. These are work
log evidence, not checks rerun in this review. Independently checked script
syntax, Make recipe expansion, existing macOS bundle layout, signatures, and
designated requirements. No implementation fixes required repeating the full
suite or starting another server or container.

Coverage gaps: macOS protected-folder access under the new holder still
requires an installed-app user check. Screenshot captures were not regenerated,
as the plan requires only recipe inspection. Linux execution evidence covers
Debian ARM64; other distributions and AMD64 remain for CI. This review did not
install the app or independently repeat the live development smoke.

## Summary

No findings. The initial review completed before writing this report; no
implementation edits were needed. The touched files remain easy to follow.

## Evidence

- `binaries` builds the backend, MCP bridge, and holder together. Both bundle
  rules call it with their executable directory, and release packaging
  archives the complete bundle. The screenshot recipe separately builds the
  same three commands beside the scripted plugin. Each build loop propagates
  failures.
- Backend discovery resolves its real executable before choosing sibling
  programs. The Linux bundle test now starts through `bin/hypercube`; the
  installed test checks the actual holder command and retains its existing
  restart, reinstall, and uninstall checks. The release test checks both
  extracted helpers after installation.
- CLI links remain limited to the backend and harness plugins. Existing
  installers copy the full bundle, so neither helper needs installer or
  uninstall special cases. The inspected macOS build has all seven Go
  executables and no helper links in `bin/`.
- New agent launches record the separate MCP path, while the plugin's saved
  legacy-command fallback and backend compatibility commands remain intact.
  Holder attachment uses its existing directory protocol. Development crash
  cleanup recognizes both command forms, without changing holder ownership
  or shutdown behavior.
- macOS signing gives the holder the backend's identifier. Independently
  inspecting the existing build found identical designated requirements for
  both: identifier `com.github.kkestell.hypercube` and the same signing
  certificate leaf hash. Deep, strict bundle verification passed. Effective
  protected-folder access remains the explicit manual coverage gap above.
- The recorded smoke departures are justified: `scripts/dev` hashes source
  contents, so changing a harmless comment exercises its rebuild trigger.
  A disposable source copy, state directory, and port avoid affecting the
  existing server. The actual panel WebSocket exercises shell launch,
  scrollback, reconnection, and stop without needing a browser interaction.

## Checks run

- `bash -n scripts/dev scripts/linux/test-bundle scripts/linux/test-install
  scripts/linux/test-release` — passed.
- `node --check scripts/screenshots/main.js` — passed.
- `make -n binaries OUT=/tmp/hc-review-dry` and `make -n screenshots` —
  passed; expanded recipes build all three commands and retain failure
  propagation and temporary-directory cleanup. No output was built.
- A Python filesystem check of `bin/Hypercube.app/Contents/MacOS` and `bin/`
  — passed: all three commands and four plugins are executable, backend and
  plugin links resolve into the bundle, and helper links are absent.
- `codesign --verify --deep --strict bin/Hypercube.app` — passed.
- `codesign -d -r-` for the bundled backend and holder — identical designated
  requirements, as described above.
- `git diff --cached --check` — passed before and after adding this review.

## Verdict

Approve. No fixes or unresolved review findings. The implementation meets the
authorized packaging, signing, cleanup, test, and documentation requirements.
The installed-app protected-folder check remains unverified and must be
reported when offering installation; running agents and shells use the new
launch commands after they restart.
