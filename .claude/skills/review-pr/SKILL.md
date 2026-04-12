---
name: review-pr
description: Run the full agent team review on a PR or current branch. Spawns code-quality-reviewer and integration-tester in parallel, then pr-quality-reviewer to consolidate.
---

Run the full agent team review on $ARGUMENTS.

## Setup

1. If `$ARGUMENTS` is empty or `--branch`, treat the **current branch** as the target. Resolve the PR number with `gh pr view --json number -q .number 2>/dev/null` (may not exist yet — that's OK, just review the local diff).
2. If `$ARGUMENTS` looks like a number, treat it as the PR number. Run `gh pr checkout $ARGUMENTS` if you're not already on that branch.
3. Identify the changed files and the base branch with `git diff --stat origin/main...HEAD` (or whatever the PR's base branch is).
4. Identify the PR title, description, and current CI status if a PR exists: `gh pr view <num> --json title,body,statusCheckRollup`.
5. Briefly summarize the scope to the user: branch, base, file count, lines changed, PR number (if any).

## Round detection (do this before pre-flight)

Determine whether this is a **first review** or a **re-review** of a branch that was already reviewed:

```bash
# Look for a previous review summary in the PR conversation (posted by Claude)
PREV_REVIEW=$(gh api repos/anstrom/scanorama/issues/<PR>/comments \
  --jq '[.[] | select(.body | test("## PR Review:"))] | last | .body' 2>/dev/null)
```

If `PREV_REVIEW` is non-empty, this is a **re-review**. Extract:
- The list of **blockers** that were flagged (lines under `### 🔴 Blockers`)
- The list of **should-fix** items
- The **"Verified" section** (what already passed in round 1)

Pass this context to both Phase A agents so they know:
1. What was already verified and should be treated as a **regression baseline** (only re-test if those subsystems changed)
2. Which specific blockers must now be confirmed fixed (highest priority)
3. What is genuinely new since the last review commit

**If no previous review comment exists** (first review): run the full review as normal.

**For re-reviews**, scope each agent accordingly:
- **code-quality-reviewer**: Focus on (a) verifying each prior blocker/should-fix is now resolved, (b) any new code in commits added since the last review, (c) regressions in previously-clean code. Do NOT re-audit code that was already clean and hasn't changed.
- **integration-tester**: Skip smoke tests for subsystems that passed in round 1 and have no new commits touching them. Focus on: (a) verifying the specific fixes for previously-flagged blockers, (b) regression spot-check of the changed subsystems only, (c) any new subsystems added since round 1.

## Pre-flight: swagger drift + test coverage audit (do this before Phase A)

Before launching agents, run `git diff --stat origin/main...HEAD` to get the file list, then:

### Swagger drift check
Run `make docs` and then `git diff --exit-code docs/swagger/ frontend/src/api/types.ts`.
- If the diff is non-empty: **blocker** — the swagger spec is out of sync. List the files that changed and instruct the author to commit the updated output.
- This check is mandatory even if no handler files changed (swagger_docs.go may reference types from other packages).

### Codecov annotations (if a PR exists and CI has run)
Run: `gh pr checks <PR number> --json name,conclusion,detailsUrl | jq '.[] | select(.name | test("codecov"; "i"))'`

Then fetch the Codecov PR comment to read patch and project coverage:
`gh api repos/anstrom/scanorama/issues/<PR number>/comments --jq '[.[] | select(.user.login == "codecov[bot]")] | last | .body' 2>/dev/null`

From the comment, extract:
- **Patch coverage** — the percentage of *new/changed lines* that are covered. Codecov threshold is ≥40% (informational — does not block CI, but flag if below).
- **Project coverage** — total coverage delta vs base branch. Threshold is must not drop >2% (blocking — CI fails if it does). Flag if the project check shows a negative delta.
- **File-level coverage table** — list any changed files where coverage dropped significantly (>5 percentage points) vs the base branch. These are should-fix items even if under threshold.
- If CI is still running (no Codecov comment yet): note "Codecov pending" and skip this check — do not block on it.
- If there is no PR yet: skip this check entirely.

### Test coverage audit
Read the PR test plan and the diff, then answer these questions:

1. **Test plan vs. automation**: For each item in the PR test plan, decide whether it describes behaviour that *should* have a unit test or integration test (e.g. "endpoint X returns Y", "function Z handles edge case W"). If yes and no corresponding test exists in the diff, list it as a coverage gap — this is a blocker.
2. **New code coverage** — apply these rules strictly:
   - Every new exported function or method must have at least one test covering the happy path.
   - Every non-trivial error path (error return, early return, branch that changes observable behaviour) must have a test that exercises it.
   - Every new HTTP handler must have tests for: 200 success, 400 bad input, and the primary error path (404 or 500).
   - Every new DB repository method must have at least one unit test or integration test.
   - Absence of tests for non-trivial new code is a **blocker**, not a nice-to-have.
   - Tautological tests (asserting language invariants or restating constants without exercising real logic) do not count as coverage.

Summarise the coverage gaps before launching the agents, so the user sees them immediately regardless of how long the agent reviews take.

## Phase A — Parallel reviews

Launch **two agents in a single message** (parallel tool calls). Each gets a self-contained prompt — they cannot see each other's context.

**Agent 1: `code-quality-reviewer`**
- Task description: "Code quality review of <branch> vs <base>"
- Prompt must include: base branch, PR number (if any), the file list from `git diff --stat`, the user's stated intent (from PR title/body or a 1-line note), and an explicit instruction to focus on the diff only (not the whole codebase). Tell it to write its report as the final message and not to commit anything.
- Explicitly ask it to audit test coverage: for each new exported function/method/handler in the diff, check whether the diff contains tests covering the happy path and key error cases. Tautological tests (asserting language invariants or restating constants) do not count as coverage.
- **For re-reviews**: include the previous review's blocker/should-fix list and instruct the agent to: (1) confirm each prior item is fixed or still open, (2) review only code changed since the last review for new issues, (3) skip re-auditing previously-clean code that has not changed.

**Agent 2: `integration-tester`**
- Task description: "Integration test of <branch> against live dev env"
- Prompt must include: base branch, PR number, the file list, which subsystems the diff touches (so it knows which CLI commands and API endpoints to prioritize), and an explicit instruction NOT to run `make dev-nuke` or destructive cleanup. Tell it to use the existing dev environment if one is already running.
- **For re-reviews**: include the previous round's "Verified" section and instruct the agent to skip re-testing those items unless the relevant subsystem has new commits. The agent should focus on: (a) verifying previously-flagged blockers are fixed, (b) regression spot-check of changed subsystems only. State explicitly which tests to skip to avoid re-running what already passed.
- **Cleanup requirement — include this verbatim in the prompt:** Before starting, record the current counts of scan jobs, profiles, and any other mutable resources that the tests will touch (e.g. `SELECT status, COUNT(*) FROM scan_jobs GROUP BY status`). After all tests complete, delete every resource the test session created: scan jobs queued during the session, profiles created during the session, discovery jobs, etc. Report the before/after counts to confirm cleanup. If a resource cannot be deleted via API (e.g. the DELETE endpoint doesn't exist), delete it directly via the database. Leaving test artifacts in the dev environment is a bug in the test run, not acceptable collateral.
- **Scan failure reporting — include this verbatim in the prompt:** If scans fail, report the *specific error message* from the database (`error_message` column in `scan_jobs`), not just "expected without sudo". Distinguish between: (a) permission failures (raw socket / sudo required — acceptable in dev), (b) queue-full failures (means the test hammered the queue — flag as a test design issue), and (c) any other error. Never describe failures as "expected" without quoting the actual error and confirming it matches the known dev limitation.

Run both in the foreground, in parallel, in the same tool-call message. Wait for both to return.

## Phase B — Consolidation

Once both reports are back, launch the third agent **sequentially** (it needs the outputs of the first two):

**Agent 3: `pr-quality-reviewer`**
- Task description: "Final PR review with cross-agent findings"
- Prompt should include:
  - Branch + base + PR number
  - The full reports from `code-quality-reviewer` and `integration-tester`, clearly labeled
  - The current CI status from `gh pr view ... --json statusCheckRollup` (if a PR exists)
  - An instruction to **deduplicate** findings across the two upstream reports, **resolve conflicts** when the two agents disagree, and produce a **single prioritized merge-or-block decision**.

## Phase C — Final report to the user

Print a single consolidated summary in this exact shape:

```
## PR Review: <branch or #PR>

**Scope**: <N files, +X/-Y lines, base: main>
**CI status**: <pass/fail/pending or "no PR yet">

### 🔴 Blockers (must fix before merge)
- ...

### 🟠 Should fix
- ...

### 🟡 Nice to have
- ...

### ✅ Verified
- Unit tests: <pass/fail>
- Lint/format: <pass/fail>
- Live API smoke test: <pass/fail>
- Cross-layer contract check: <pass/fail>
- Codecov patch coverage: <X% / pending / no PR>

### Recommendation
<merge | merge after fixing blockers | block — reason>
```

Then print a one-line pointer to where the user can read each agent's full report if they want detail (or offer to print any of them in full).

## Rules

- **Never commit, push, force-push, or merge anything** as part of this command. Read-only.
- **Don't modify config files** or run `make dev-nuke`.
- If the dev environment isn't running and the user is on a feature branch, ask before running `make dev` (it requires sudo).
- If `code-quality-reviewer` and `integration-tester` produce contradictory findings, surface the contradiction explicitly in the consolidation rather than picking one silently.
- If there's no PR yet (just a local branch), still run the review — just note "no PR yet" in the CI status line.
- **Check that every commit addresses exactly one concern.** If `git log origin/main..HEAD` reveals commits that bundle unrelated changes (e.g., two distinct bug fixes in one commit), flag it as a blocker — the branch must be split before merge.
- **Check the PR test plan for CI-redundant items.** Items like "`go test ./...` passes" or "`golangci-lint` — 0 issues" are covered by CI and must not appear as manual checkboxes. Flag any such items as should-fix; test plans must only list live/manual verification steps that CI cannot perform.
- **After making any code changes as part of the review process, update the PR title and body** using `gh pr edit` so they accurately reflect the current branch state. Summary, new endpoints, and test plan items must match what is actually in the branch.
- **Test plan items that describe automatable behaviour must have tests.** If a test plan item says "endpoint X returns Y for input Z" or "function handles edge case W", that behaviour belongs in a unit or integration test — not just a manual checkbox. Flag any such item that has no corresponding test in the diff as a blocker.
- **Swagger drift is a blocker.** Run `make docs` and `git diff --exit-code docs/swagger/ frontend/src/api/types.ts` — any diff means the spec is stale. The pre-push hook catches this locally; a CI failure here means the author skipped the hook.
- **Thorough test coverage is required, not optional.** Low coverage is a blocker. Every new handler, service method, and repository method needs tests. "We'll add tests later" is not accepted.
- **Do not re-verify in round 2+ what was already clean in round 1 and has not changed.** The cost of a review is proportional to how much it teaches — re-running identical tests that already passed adds noise without signal. Each round should advance the review, not repeat it.
