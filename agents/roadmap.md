# Roadmap

## Installation, upgrade, and recovery validation (deferred)

Complete the checks beyond the Codex exercises already performed:

- Install from a fresh macOS account and authenticate a harness for the first
  time. The existing exercises used copied credentials.
- Check Finder launch, Gatekeeper approval, and visible UI interaction with
  the packaged app.
- Exercise first use and native conversation recovery with Claude Code,
  OpenCode, and Pi.
- Exercise an upgrade that changes the database schema, then restore the
  pre-upgrade backup with its matching previous build. The existing live
  upgrade and recovery exercise used an unchanged schema.

## Browser visual verification

Check the development UI for any remaining padding or hover background on
collapsed tool calls. The disclosure removes both in commit `7055d4e`; its
current classes and generated styles still do so. Visual confirmation remains
with the user.

## Completed UI work

- Answered questions use labeled cards with the selected answer. Exact text
  copies of structured questions and their choices are omitted from the
  transcript, including saved conversations.
- Expanded tool arguments and output share one container with section headers.
- Transcript and composer gutters shrink together in narrow panes; expanded
  "Worked for" groups have more indentation.
- Empty workspaces offer Add Workspace without an empty tab-list artifact.
- File highlighting uses GitHub Light Default and Dark Default. Native browser
  previews display images, audio, video, PDF, and HTML without the text size
  limit. Textual previews retain source mode within that limit.

## Usage windows (investigated)

The reported Codex account supplies only a seven-day primary window, with no
secondary window. The stored reading likewise contains only seven days. There
is no five-hour reading available to show. Keep displaying the windows the
harness reports; when both are present, show the shortest normally and the
longest on hover or keyboard focus. Tests cover both-window and weekly-only
readings. Revisit if Codex supplies a five-hour window that does not appear.

## Codex MCP tool approvals offer Always allow

Codex's MCP tool approvals arrive as `mcpServer/elicitation/request` without
the tool's name or the elicitation's own ID, so the adapter has no key to save
and offers no Always allow for them.

Offer Always allow once Codex includes a stable tool identifier in the request.
The installed Codex 0.166.0 protocol schema still omits it (checked 2026-11-04).
