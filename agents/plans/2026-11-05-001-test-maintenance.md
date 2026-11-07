# Reduce test maintenance

Keep persistence, restart, delivery, protocol translation, external-input
validation, and consequential user-interaction tests. Remove redundant layers
and low-value presentation matrices without recreating their assertions.
Production behavior stays unchanged; active sessions need no restart.

- Keep the real app-process integration test; delete the parallel in-process
  app suite and its private HTTP/MCP helpers.
- Delete the harness Client wrapper suite and unused stand-in commands. Keep
  holder, decoder, plugin-protocol, and runtime integration tests.
- Remove UI server tests that replay session semantics through thin adapters;
  retain actual socket transport, file routes, and origin validation.
- Remove component suites for thin view/menu/file-panel wrappers and simulated
  layout. Keep state ownership tests and selected destructive-action, draft,
  streaming, input-safety, and request-interaction tests. Trim the large App,
  Sidebar, and Transcript suites to those responsibilities.
- Delete plugin bookkeeping tests and duplicate process smoke machinery. Keep
  protocol-specific fixtures, startup/resume failures, and RPC/process tests.
- Consolidate AGENTS.md guidance around choosing test responsibilities before
  preserving individual assertions. Reuse these plan/review files.

Accept reduced automated coverage of presentation, simple UI wiring, and exact
wrapper diagnostics. Review the resulting suite and run repository checks;
add no replacement test framework or warning-cleanup project.
