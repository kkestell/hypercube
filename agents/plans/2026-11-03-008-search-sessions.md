# Search sessions

## Goal

Search session titles and saved conversational text across all workspaces, and open a result from a Command dialog.

## Decisions

The least costly complete approach is a server query over existing sessions and transcript JSON, with no schema or index. User and agent text items and the message body of outgoing `send_message` items count; other tool fields, output, and reasoning do not. Return one result per matching session, with a conversation snippet when applicable. The existing session selection action handles stopped and ended sessions.

## Implementation plan

- Add a store query for sessions and matching transcript text and expose it through a UI GET route. Match case-insensitively and return title, workspace and a short matching excerpt.
- Add the stock Command dialog to the sidebar, with a header search button and Command+K, async search, keyboard selection, and existing session navigation.
- Update `docs/usage.md` for search.

## Test plan

- Verify title and conversation matches across workspaces and session states, exclusion of tool and reasoning text, and matching snippets through the server route.
- Verify opening the dialog, searching, and selecting a result through browser behavior.
