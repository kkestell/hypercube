# Backend composition plan assessment

## Scope and coverage

Parent assessment of [plan 010](../plans/2026-11-07-010-backend-composition.md)
against command dispatch, backend startup and shutdown, alerts, browser route
registration, host and origin checks, and their existing tests.

## Summary

Composing the browser, MCP, desktop, and version routes in the
backend command gives the shared access checks their proper owner. The UI
package then owns only browser behavior. Moving existing startup and shutdown
code into files by concern keeps lifecycle ownership and ordering unchanged.

Keeping the current handler would leave browser code responsible for unrelated
backend clients. A separate backend package would add an exported interface
for one command. The proposed direct composition avoids both costs.

Implementation exposed one detail the original plan missed: nesting the
browser mux changes the `Allow` header for unsupported methods on `/mcp`
from `GET, HEAD, POST` to `GET, HEAD`. The implementation agent reproduced
that difference. Tracing both mux registrations confirms the missing POST
comes from the outer mux. A local `/mcp` fallback preserves that header and
delegates GET and HEAD to the browser handler; the regression test checks
both behaviors. This preserves the plan's requirements without a new routing
abstraction.

## Checks run

Inspected the current command, UI handler and route tests, and the plan.
Compared method routing and fallback through the browser file server, and
traced restore's HTTP startup callback and shutdown order. No code checks were
needed for this assessment; implementation must preserve those behaviors and
run the planned tests and `make check`.

## Verdict

Proceed within plan 010 after plan 009. Preserve the shared access checks,
route behavior, and lifecycle order without adding a new backend abstraction.
