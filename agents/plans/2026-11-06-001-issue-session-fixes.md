# Session restoration, shutdown, and requests

## Goal

Resolve #12, #14, #17, #18, and #20 within the existing session and holder owners.

## Decisions

- Restore registers every saved launch under the lifecycle lock before starting
  HTTP through a callback. HTTP then remains available while readers attach,
  shells adopt, and queued messages deliver. This closes the admission race
  without a second lifecycle state or a separate request gate.
- Detach closes connections without taking the sender lock. Holder.Write keeps
  its lock only while obtaining its connection so Close can interrupt socket
  writes; the existing session sender still orders commands.
- Saved offers belong to their transcript, survive parking, and appear beside
  live plugin requests. Read them directly from storage; do not duplicate them
  in live pending items or use the attention flag that prevents parking.
- Removal helpers claim family membership and record completed members; the
  existing function retains visible preflight, shutdown, recheck, and removal
  order. No new lifecycle owner or cleanup framework.

## Implementation plan

1. Split registration from restoration actions in internal/session/restore.go;
   start HTTP at the registration boundary in cmd/hypercube/main.go.
2. Remove sender-lock acquisition from Detach in sessions.go; shorten the
   connection lock in internal/hold/hold.go. Add a blocked-writer regression.
3. Seed every session's reply position on alert watcher startup in alerts.go.
4. Extract direct family claim and persistence helpers in removal.go.
5. Add internal/store/offers.go to query open offers from active sessions;
   include them in Requests in transcript.go, returning read errors through
   alerts.go and internal/ui/socket.go. Derive UI waiting indicators from the
   request list in sidebar/session-item.tsx and sessions/selection.ts.

## Test plan

- A controlled request at Restore's HTTP callback sees all saved launches and
  keeps their original identities after restoration completes.
- Detach releases a sender writing to a nonreading plugin, leaves its holder
  alive, and retains final checkpoints.
- A watcher started during a question does not announce the preceding reply
  when the question closes; a subsequent reply still alerts.
- Existing removal tests retain dirty refusals and partial-child progress.
- Offers appear in requests while running, parked, and restored; answering or
  archiving removes their indicators; new offers alert once. Waiting counts
  and sidebar status include parked offers.
