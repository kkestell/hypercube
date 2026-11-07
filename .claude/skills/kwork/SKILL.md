---
name: kwork
description: Implement a Hypercube plan from `agents/plans/`, write a work log in `agents/work/`, and stage the related changes for review.
argument-hint: "[plan path or slug]"
---

# Work

Use this skill when the [development workflow](../../../agents/development.md)
assigns a plan or when the user asks to implement one.

1. Resolve `$ARGUMENTS` to a plan in `agents/plans/`. If no plan is named,
   inspect the plans and repository evidence to identify an unimplemented one.
   Proceed if exactly one is clear; otherwise ask the user which plan to use.
2. Read the plan and the files it references. Carry out its tasks. If evidence
   changes the plan, update it and record the departure in the work log.
3. Run the checks required by the plan and
   [development](../../../agents/development.md). Resolve failures before
   reporting completion.
4. Read `agents/work/_template.md` if present, otherwise
   [assets/_template.md](assets/_template.md). Write a concise work log in
   `agents/work/` as `YYYY-MM-DD-NNN-slug.md`, using today's date, the next
   sequence for the day, and the plan's slug. Record departures, decisions,
   checks, manual verification, and follow-up work without repeating the plan
   or diff.
5. Stage only the implementation, work log, and any plan update. Do not commit.
6. Report the result, any departures from the plan, work log path, staged
   files, and next step: `/kreview staged changes for <plan path>`.
