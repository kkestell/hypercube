# Plan title

Write the plan after making the main decisions. Make the change executable
at the level of detail established by comparable repository plans. Specify
contracts and coordinated edits; leave routine coding choices to the
implementer. Include only applicable sections and delete placeholder text,
including this guidance.

## Goal

State the concrete problem, resulting behavior, scope, and prerequisites.

## Related code

- `path/to/file` — Name the relevant functions or types, their current
  behavior, and why they change. Identify affected callers and test fixtures
  where the work crosses files or packages.

## Decisions

Describe the chosen implementation, rather than listing topics for the
implementer to investigate. Specify affected APIs, data shapes, ownership,
lifecycle, configuration, and compatibility as applicable. Use signatures,
examples, tables, or code sketches where they make the decision unambiguous.
Explain how the changed parts work together and which existing behavior must
be preserved. Record unresolved consequential decisions explicitly.

## Test plan

- For meaningful behavior and regressions, identify the test boundary,
  fixture or setup, inputs or actions, and observable expected results.
- Identify existing tests to move or update and any necessary new coverage.
  Include relevant check commands and platform validation, proportionate to
  the change.

## Implementation plan

- Order concrete edits by dependency. Name the files and functions to change
  and explain the change to each.
- Spell out coordinated caller and fixture changes; an instruction to
  "update all callers" is insufficient when their wiring needs decisions.
  Include removal of the code and setup replaced by the change.

## Documentation updates

- Name the existing documents and the facts or links that must change.
  Omit this section if there are none.
