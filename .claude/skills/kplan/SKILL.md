---
name: kplan
description: Plan a Hypercube code or behavior change in `.agents/plans/` when the development workflow or user calls for a plan. Do not use for standalone documentation edits.
argument-hint: "[feature idea, bug report, or change to plan]"
---

# Plan

Use this skill when the [development workflow](../../../docs/dev/development.md)
calls for a plan or when the user asks for one.

1. Read the request and `$ARGUMENTS`. If the change to plan is unclear, ask the
   user what they want to plan.
2. Read comparable plans in `.agents/plans/` to establish the repository's
   level of implementation detail. Inspect the relevant code, behavior,
   callers, and test fixtures. When revising existing work, use findings
   already established and investigate gaps or changed assumptions. Brainstorm
   first when the user asks, the request is unclear, or a real decision
   remains. Ask one focused question at a time, with options, trade-offs, and
   a recommendation; wait for the answer. Resolve ordinary details from the
   repository.
3. Read `.agents/plans/_template.md` if present, otherwise
   [assets/_template.md](assets/_template.md). Write the plan in `.agents/plans/`
   as `YYYY-MM-DD-NNN-slug.md`, using the next sequence for the day. The plan
   must carry the decisions into concrete implementation instructions, so
   the implementer can begin without repeating the design investigation.
   Follow the template's guidance on contracts, coordinated edits, and
   verification; a list of concerns or desired outcomes is insufficient.
   When splitting or revising a plan, preserve the applicable decisions and
   implementation detail in the resulting plans. Keep only relevant sections;
   leave out conversation history and work for a later change.
4. Do not review the plan during this step. Stage the plan alone. Do not change
   code or commit.
5. Report the plan path, the files it expects to change, and the next step:
   `/kwork <plan path>`.
