# Plan review: 2026-11-09-006-global-pull-requests

## Round 1

- Task ID: `2026-11-09-006-global-pull-requests`
- Mode: `plan`
- Author: reviewer `review-global-prs`, Hypercube child of `04ef1b14`, Claude
  Code, native conversation `e3fb53e6-b1a8-4613-be42-9098ff5686c0`, model
  `claude-opus-5-5`. Independent of the planner (`plan-global-prs`).
- Plan (repository-relative): `.agents/plans/2026-11-09-006-global-pull-requests.md`,
  blob `a155df32132832486e622157be555d40263f961f`
- Guidance blobs: none (no linked shared designs)
- Source HEAD: `73f6779e5adaaafb0ac867b7e101d33b2396e04c`

### Verdict

Work size: right-sized, at the large end. Detail: appropriate, with one
verification gap and two unsettled edges. Not ready until finding 1 is
corrected; findings 2 and 3 are small decisions for the planner. Action:
`resolve-decisions`.

### Findings

1. **Existing fixtures break under URL keys and the repository filter
   (required).** Verification says "Keep `TestPullRequestsDescribeTheAccountsPart`
   passing with the new stand-in", but that test's `node()` helper
   (`internal/github/watcher_test.go`, around line 350) sets no `url` and no
   `repository`. Under the plan, every node then has URL `""` (the dedupe
   collapses them to one) and `Repo` `""` (the workspace filter drops them),
   so the test cannot pass by changing only the stand-in. The same holds for
   `TestPullRequestsMapAndKeepOnFailure` (no `repository`). Correction: state
   that fixture nodes gain `url` (unique per node) and
   `"repository": {"nameWithOwner": "o/r"}`, that `SetRequiredFailures(1)`
   becomes the node's URL, and that the not-a-pull-request guard in the read
   loop becomes `n.URL == ""` (today `n.Number == 0`). Without this a Sonnet
   implementer may restore number keys to make the old test pass.

2. **"Each login gets its own list" is not testable with the planned stand-in
   (minor).** `githubtest.API` answers the pull request query identically for
   every token, and the planned `SetPullRequests` takes no token. The test
   "Two logins each get an account with its own list" can only show both
   accounts receiving the same list. Either make the answer per token
   (`SetPullRequests(token, …)`, the stand-in already keys permissions by
   token) or reword the test to what it shows: both logins read, the same URL
   appears under both.

3. **Home view with zero workspaces is ambiguous (minor).** Step 8 renders the
   global view "in the home case, before the existing no-workspaces
   invitation", and the sidebar button appears whenever `accounts` is
   non-empty. With accounts but no workspaces, it is unclear whether the
   global view replaces the "Add a workspace to get started" invitation,
   sits above it, or is hidden. Recommendation: with no workspaces, keep the
   invitation alone and hide the button; otherwise the home case shows the
   global view.

### Answers to the coordinator's questions

- **Query cost.** Re-measured live with `rateLimit(dryRun:true)` against
  kkestell using the plan's exact fragment: 32 points at involved 50 /
  requested 20 (matches the plan); 23 at 30/20; 18 at 20/20; 27 at 50/10.
  Current open counts for the involved search are 18, 19, and 10 for the three
  accounts, and the hour's remaining budget was 4,141, 4,933, and 4,145 of
  5,000 at the time of measurement. About 1,920 points an hour per account
  leaves roughly 2,000–3,000 for agents and gh on the same token. Acceptable as
  planned; the user asked for about 50. If headroom matters later, `first:30`
  cuts a quarter of the cost and still exceeds every account's current count.
  Not a required change.
- **Dropping the per-repository open count.** Sound. Keeping it needs either a
  per-workspace query (what the request removes) or per-repository aliases in
  each account's query, which forces the pass to resolve workspace
  repositories before reading accounts. The always-shown "All open pull
  requests" link keeps the navigation. It is a visible change the user did not
  ask for, so report it when the work lands.
- **Home-view placement.** Coherent with the selection model. `selected ===
  null` already means "nothing shown" (`openUnsent`, archiving the shown
  family, and a deleted workspace dashboard all set it), and today that state
  renders an empty center, so the global view fills an existing state rather
  than adding one. `attach` clears `editor` when `selected` is null, so the
  `aria-pressed` condition is well defined. `view-storage.ts` restores a saved
  `selected: null` when the URL has no hash, so the home view also survives a
  reload; without a hash and with a saved session, today's behavior holds.
  Only the zero-workspace case (finding 3) needs settling.
- **Session pull request icons.** Unaffected. `internal/ui/snapshot.go`
  matches `!pr.Fork && pr.Branch == branch` over `Status.PullRequests`, which
  the plan still fills by filtering the account's list to the workspace's
  repository. The only new exposure is the 50-result cap shared across
  repositories: an account with more than 50 involved open pull requests could
  lose an old one in a quiet repository. Current counts (10–19) are far below
  that.

### Checked and sound

Error rule (account error added to a workspace only when the workspace's own
token read succeeded) matches the per-pass token cache, which caches only
successes. `$login` stays declared for the `reviews(author:$login)` field. The
stand-in's dispatch by `involved:search` does not collide with the permission
query or the `isRequired` query, so `Requests()`-based tests keep counting only
permission queries. `TestPendingChoiceDoesNotFetchOrQuery` keeps holding for
the workspace (empty list, `Updated` 0) even though accounts are now read.
Repository aliases in `requiredFailures` with `%q` quoting are safe for GitHub
`nameWithOwner` values.

### Normalization

- Active target: 600–1,500 words (default band from `normalization.md`; no
  curated project references).
- Measured: 1,763 words, 78 in fenced code (`plan_metrics.py`).
- Comparison: one mechanism (per-account reads) carried through backend, wire
  format, and three browser surfaces. That is larger than the "one operator
  family through existing layers" reference but not a family of separate
  mechanisms; the browser surfaces all consume one new `accounts` field and one
  `yourMoves` helper. The plan's own split (backend through step 4, browser
  5–9) is available if the implementer stalls, but splitting now adds a
  wire-format handoff without removing decisions. The excess over the band is
  the measured cost evidence and the three UI placement decisions the request
  asked to settle, not implementation dumps. No trim required.
- Confidence: moderate; the band is provisional.

### Handoff

To pass: correct finding 1 in the Verification section (fixture fields, URL
guard, required-failure URL). Settle finding 2 (per-token stand-in or reworded
test) and finding 3 (zero-workspace home case). The parent can verify these
corrections without another independent round.

### Checks

- `gh api graphql` (read-only, `rateLimit(dryRun:true)` and `issueCount`
  searches) against all three accounts, 2026-11-09.
- `plan_metrics.py --plans .agents/plans --plan …` (run).
- No builds or tests (plan review).

### Limitations

Did not inspect `sidebar/workspace-section.tsx` markup or `data-socket.ts`
beyond confirming the fields named exist; those are routine.
