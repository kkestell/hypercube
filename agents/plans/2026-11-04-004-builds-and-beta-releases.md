# Identifiable builds and beta releases

## Goal

Identify checkout builds and official beta builds, run the existing checks in
macOS CI, and prepare downloadable ad hoc signed app bundles. Preserve the
user's parallel work and `make install-service` workflow.

## Decisions

Go already embeds the module version, commit, and dirty state. Read that
metadata for the CLI, running app, and bundle information instead of adding
linker variables or generated version files. Keep the browser asset hash for
its existing reload behavior.

Normal builds and installations continue accepting uncommitted work and do not
run checks automatically. Only release packaging requires a clean beta tag.
The initial download targets Apple Silicon, matching the machine currently
used and validated; do not claim Intel support without building and checking it.

Reuse `make bundle` and its ad hoc signing. Package a ZIP and checksum, with no
Apple credentials or notarization. A pushed beta tag runs checks and packaging
before creating a draft GitHub prerelease for the user to inspect and publish.
Do not push a tag or create a remote release during implementation.

A downloaded app must launch from Finder. Its no-argument invocation should
re-exec through the login shell, as the service already does, so installed
harnesses are found. Explicit CLI commands and service lifecycle stay as they
are. Include the existing service installer in the bundle for binary installs.

## Implementation plan

1. Add a small build-information package using `runtime/debug`, expose
   `hypercube version`, and include the identity in the startup log.
2. Serve the running process's identity at `/version` and show it in Settings.
   Keep it independent of the browser asset hash.
3. Populate bundle metadata from the built executable, enable Finder startup,
   and include the existing service installer beside the executable.
4. Add release packaging with exact beta-tag/clean-tree checks, signature and
   identity verification, a ZIP, and SHA-256 checksum. Keep release restrictions
   outside local build and installation targets.
5. Add macOS checks and tag-triggered draft prereleases using the pinned Go and
   Bun versions from the repository. Document downloads, Gatekeeper's Open
   Anyway procedure, binary service installation, version reporting, and release
   preparation in the existing installation guide.
6. Obtain an independent findings-only review. Leave unrelated changes, the
   index, commits, running sessions, and the installed app alone.

## Test plan

Check CLI and running-server identity, missing metadata, and Settings display.
Build from isolated clean, tagged, and modified Git trees to verify Go metadata
and release restrictions without modifying the real checkout's history. Verify
bundle metadata, signatures, ZIP contents, and checksum. Run `make check` and
use the isolated development server for the Settings change. GitHub-hosted
execution and downloaded-app Gatekeeper approval require external validation;
report those limits explicitly.
