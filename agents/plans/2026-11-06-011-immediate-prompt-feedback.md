# Show submitted prompts immediately

## Goal

For issue #34, clear the composer and show each submitted user message,
including its images, immediately for new and existing sessions. If sending
fails, show the failure and retain the complete message for retry, including
when the user has already typed another draft or switched sessions.

## Decisions

The composer already clears synchronously. `Runtime.Input` waits for the
launch and plugin reply; `commands.go` reserves the user item's position but
only commits the item to the transcript after plugin acceptance. `Create`
also waits for Input before the HTTP response identifies the new session.
Returning early from the backend would require changing launch and command
completion ownership and exposing later failures. Instead, keep those
semantics and show a page-owned provisional submission while the existing
request runs. This adds no durable queue or new transport.

Use a client-generated submission ID to reconcile the provisional row with
the authoritative user item, not text or timing (identical prompts may be
sent). Carry it only through the existing HTTP user-submission path, validate
it as external input, and use it for the corresponding host user item. Other
input paths retain their existing IDs and behavior. A transcript item with the
same ID replaces the provisional row even if it arrives before the HTTP
response. Keep provisional state outside the attached transcript, which is
recreated on switching sessions; associate new-session state with the existing
unsent identity and transfer it to the real session when its ID is known.

A failure retains its own text and images with an explicit retry action; it
must not overwrite or merge with a newer composer draft. A failed creation can
already have persisted a session before Input fails: return that session ID
with the failure when available, and retry input in that session rather than
creating another one. Preserve the current behavior for failures before a
session exists. A transport failure or timeout can be ambiguous: show server
failures as returned, and explain that transport failures leave delivery
unconfirmed. Keep retry available without classifying delivery or adding
idempotent delivery or recovery guarantees.

## Implementation plan

1. In `internal/ui/web/sessions/session-actions.ts` and a focused submission
   state module, create the provisional row and clear the draft in one store
   update before awaiting the request. Store submissions by session or unsent
   identity in `state.ts`, independently of attached view state. Move the
   first submission and any newer draft to the created session without
   changing the user's current selection after a switch. Keep failed entries
   until retried or otherwise explicitly dismissed; ensure retry cannot send
   twice from repeated clicks.
2. In `internal/ui/actions.go`, `internal/session/input.go`, and
   `internal/session/sessions.go`, thread the validated correlation ID only
   for HTTP user input and creation, preserving existing callers and command
   acknowledgment behavior. Return enough structured creation result to
   distinguish a pre-creation refusal from a failed first input after session
   creation. Adapt the submission request's error handling in `api.ts` as
   needed without changing other actions.
3. Compose provisional rows with the displayed transcript in
   `transcript/transcripts.ts` and `transcript/transcript.tsx`, including the
   unsent placeholder. Reconcile by ID on incremental and reset transcript
   messages regardless of response order. Show a clear failure and retry beside
   the affected message, using the original text and images. Preserve normal
   transcript order, scrolling, selection, and existing message rendering.
   Clean up submission state on success and session removal without dropping
   a pending or failed submission on a routine view switch.

## Test plan

- Exercise first and follow-up submissions with deferred HTTP/plugin replies:
  composer clears and the text and images appear before completion. Verify
  authoritative transcript before and after the HTTP response, including a
  reset and identical consecutive prompts, yields one row per submission.
- Refuse input and creation before and after session creation; verify visible
  failure, retry with original text and images, and preservation of a newer
  draft. Cover network failure, uncertain timeout, and no repeat retry while a
  retry is in flight.
- Switch sessions or workspaces during pending and failed sends, then return;
  verify no message is lost, no unrelated selection is taken over, and first
  submission transfers to the created session. Cover newly typed draft and
  pending image reads using existing session-action tests.
- Add focused HTTP/runtime coverage for validated IDs and successful
  authoritative item correlation, retaining existing other input behavior.
  Run the relevant UI and Go tests and `make check`.
