# Runtime boundary cleanup: sequence overview

This is an overview, not an implementation plan for `/kwork`. Each linked
plan has a focused source responsibility and its own review and verification.
The [architecture review](../reviews/2026-11-07-007-runtime-boundaries.md)
records the findings and the comparison with a package-only cleanup.

## Implementation order

| Order | Plan | Complete result |
| --- | --- | --- |
| 1 | [Application configuration](2026-11-07-006-application-config.md) | Preferences and upgrades have their own package; callers and tests use it. |
| 2 | [Harness protocol boundary](2026-11-07-007-harness-protocol-boundary.md) | Shared harness data is independent of backend discovery and process clients. |
| 3 | [MCP bridge executable](2026-11-07-008-mcp-bridge-executable.md) | Bridge transport has its own package; new plugin launches use the helper and the old command shares its implementation. |
| 4 | [Holder executable](2026-11-07-009-holder-executable.md) | Agents, shells, and tests launch a real holder helper through explicit paths. |
| 5 | [Backend composition](2026-11-07-010-backend-composition.md) | Focused backend files own HTTP assembly and resource lifetime; UI owns browser routes. |
| 6 | [Runtime integration](2026-11-07-011-runtime-integration.md) | Build, development, bundles, signing, installation, and current documentation support the complete executable layout. |

Each plan assumes the preceding results have been implemented and reviewed.
The source plans own necessary fixture construction and focused behavior
checks. Intermediate bundles and normal development recipes need not supply
the new helpers until the final integration plan. These are implementation
boundaries, not releases; do not add temporary production fallbacks or repeat
platform packaging checks merely to make each intermediate bundle shippable.
The final plan owns distribution and operational verification of the combined
result. Existing required checks before code commits still apply.

## Constraints across the sequence

- The backend remains the SQLite and session-runtime owner. Keep its
  coordinated transactions, replay, lifecycle, and terminal admission together.
- Preserve saved command bytes and IDs, holder files and adoption, native
  conversations, environment, working directory, and existing delivery and
  exit semantics. No schema migration is required by these changes.
- Keep native harness configuration per launch. Leave user settings,
  credentials, and `HOME` alone.
- New launches ultimately use private sibling `hypercube-mcp` and
  `hypercube-hold` executables. Keep `hypercube mcp` and `hypercube hold`
  calling the shared implementations directly for existing callers. The
  backend already uses their dependencies; compatibility needs no helper
  lookup or forwarding process. Keep installer ordering and existing
  replacement gaps unchanged.
- Keep backend assembly in focused `cmd/hypercube` files. Do not introduce
  `internal/backend`, generic program registries, new IPC, version negotiation,
  or stronger recovery, supervision, or delivery guarantees.

Start with `/kwork agents/plans/2026-11-07-006-application-config.md`.
