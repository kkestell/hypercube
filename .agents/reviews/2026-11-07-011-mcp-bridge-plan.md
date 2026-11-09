# MCP bridge plan assessment

## Scope and coverage

Parent assessment of [plan 008](../plans/2026-11-07-008-mcp-bridge-executable.md)
against the current bridge, session launch arguments, runtime construction,
and backend startup. This assesses the boundary before implementation; code
review follows separately.

## Summary

No findings. Moving the existing forwarder into an independent package and
command removes its backend dependencies without adding a transport or
changing delivery behavior. Passing executable paths from the backend command
gives discovery one owner. The smaller baseline of extracting only the
package would leave plugins coupled to the backend command and would not meet
the plan's executable boundary.

The legacy command and shared start-argument resolver serve concrete existing
consumers: harness configurations and persisted start commands. They preserve
compatibility without duplicating the forwarding implementation.

## Checks run

Inspected the plan and current `internal/mcp/mcp.go`, runtime construction,
session start arguments, and backend startup. No code checks were needed for
this assessment.

## Verdict

Proceed within the plan. Packaging is explicitly deferred to plan 011; tests
build the helper themselves. This intermediate change is not ready to install
on its own. Do not add packaging work or install it as part of plan 008.
