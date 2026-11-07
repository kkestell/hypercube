# Transcript image links

## Scope and coverage

Reviewed commit `bdd0acc`, all thirteen touched files, the transcript store and
runtime update paths, image-producing harness code, image rendering and scroll
restoration, and provisional prompt tests.

The outer `shownItem.Images` field correctly replaces the embedded image field
in JSON. Saved image reads use both session and item IDs; pending items take
precedence over checkpointed entries under the runtime lock. Invalid indices,
missing entries, invalid base64, and unsupported MIME types take explicit error
paths. Image responses retain the local-origin protection of other app routes.

Claude, OpenCode, and Pi attach tool images at completion. Codex generated and
viewed images also arrive at completion; its item translation ignores repeated
completions. No supported flow was found that replaces an already displayed
image's bytes while retaining the same link. URL versioning would add machinery
without an evidenced failure here.

The change remains limited to image transport, retaining saved image data and
provisional prompt data URLs. Existing browser tests exercise first and follow-up
prompt images before HTTP completion, collapsed image rendering, and delayed
image scroll restoration. The new socket test exercises image links in updates
and resets, image bytes and MIME type, and an unavailable image index. Pending
image precedence and the other invalid-request branches were traced rather than
given additional tests.

## Summary

No findings. No code fixes were necessary.

## Checks run

- `make check` — passed: UI and website builds, TypeScript, 345 browser tests,
  28 desktop tests, Go build/vet/format checks, and the complete Go race suite.
- `git diff --check` — passed.

## Verdict

Approve the scoped performance improvement. This review did not repeat the
writer's timing measurements or reproduce the original reported 1–2 second
session-switch delay; neither those measurements nor this review establish that
the full reported delay is resolved. No blocking correctness, scope, or
performance regression was found.
