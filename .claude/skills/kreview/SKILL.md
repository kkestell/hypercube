---
name: kreview
description: Review Hypercube code changes or files, fix confirmed medium and high severity issues, write a review in `.agents/reviews/`, and stage the review and fixes. Do not use for plan or standalone documentation review.
argument-hint: "[review scope]"
---

# Review

Use this skill when the [development workflow](../../../docs/dev/development.md)
calls for code review or when the user asks for one.

1. Read `$ARGUMENTS` and establish the review scope. Ask for clarification only
   if the answer would change what to review.
2. Inspect that scope, its relevant callers and requirements, and the checks
   that bear on it. Confirm each finding by tracing behavior, reproducing it,
   or running a focused check. Note material gaps in coverage. Finish the
   initial review before editing the code under review.
3. Assign each finding a severity. Fix every confirmed medium or high severity
   issue in this session; do not hand those fixes back to the parent. Fix clear
   low severity issues when proportionate. If a fix needs a behavior or design
   choice the requirements do not settle, ask the user one focused question at
   a time, then apply the answer. Validate the fixes and report anything left
   open.
4. Read `.agents/reviews/_template.md` if present, otherwise
   [assets/_template.md](assets/_template.md). Write the review in
   `.agents/reviews/` as `YYYY-MM-DD-NNN-slug.md`, using the next sequence for
   the day. Give findings IDs in severity order and record scope, material
   coverage gaps, evidence, fixes, checks, and verdict. Say so if there are no
   findings.
5. Stage only the review and its fixes. Do not commit.
6. Report the verdict, findings, review path, and staged files.
