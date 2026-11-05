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

## Codex MCP tool approvals offer Always allow

Codex's MCP tool approvals arrive as `mcpServer/elicitation/request` without
the tool's name or the elicitation's own ID, so the adapter has no key to save
and offers no Always allow for them.

Offer Always allow once Codex includes a stable tool identifier in the request.
