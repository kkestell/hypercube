# Immediate prompt feedback

## Plan

`agents/plans/2026-11-06-011-immediate-prompt-feedback.md`

## Summary

First and follow-up prompts appear with their images while HTTP waits for the
agent. Each failed message retains its own Retry and Dismiss controls, separate
from newer drafts. HTTP correlation replaces provisional messages by ID without
changing backend acceptance. Failed first input returns the persisted session
ID, so retry uses that session.

## Decisions

- Server failures are displayed as returned. Only transport failures explain
  that delivery was not confirmed; no delivery classification, durable queue,
  or retry deduplication was added. The plan now states that narrower choice.
- Final caller inspection found that new sessions bypassed `Transcript` in
  `AgentPane`. The pane now shows the transcript once a submission exists,
  and the rendered regression exercises that actual entry point.
- Removed the unused draft restoration helper and tesseract fading option.

## Checks run

- `bunx tsc -p .` in `internal/ui` — passed.
- Focused session actions, selection, transcript, composer, model picker, and
  settings tests — 99 passed.
- Rendered submission tests through `AgentPane`, draft tests, and app tests —
  14 passed. Existing React `act` warnings appeared in passing app/composer
  tests.
- `GOCACHE=/private/tmp/hypercube-go-cache go test -race ./internal/session
  ./internal/ui -run 'TestHTTPSubmissionCorrelation|TestImagePrompts|
  TestInputIsTimedWhenSent|TestCreate' -count=1` — passed. The expression was
  supplied on one line without the displayed wrapping.
- `git diff --check` — passed.
- No browser or installed-service run. The parent owns the combined
  `make check` after all concurrent fixes and independent review.
