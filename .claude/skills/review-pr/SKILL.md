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
REPO=$(gh repo view --json nameWithOwner -q .nameWithOwner)
PREV_REVIEW=$(gh api repos/${REPO}/issues/<PR>/comments \
  --jq '[.[] | select(.body | test("## PR Review:"))] | last | .body' 2>/dev/null)
```

If `PREV_REVIEW` is non-empty, this is a **re-review**. Extract:
- The list of **blockers** that were flagged (lines under `### 🔴 Blockers`)
- The list of **should-fix** items
- The **"Verified" section** (what already passed in round 1)

Pass this context to both Phase A agents so they know:
1. What was already verified — treat as regression baseline, only re-test if that subsystem changed
2. Which specific blockers must now be confirmed fixed (highest priority)
3. What is genuinely new since the last review commit

**If no previous review comment exists** (first review): run the full review as normal.

**For re-reviews**, scope each agent accordingly:
- **code-quality-reviewer**: Focus on (a) verifying each prior blocker/should-fix is now resolved, (b) any new code in commits added since the last review, (c) regressions in previously-clean code. Do NOT re-audit code that was already clean and hasn't changed.
- **integration-tester**: Skip smoke tests for subsystems that passed in round 1 and have no new commits touching them. Focus on: (a) verifying the specific fixes for previously-flagged blockers, (b) regression spot-check of the changed subsystems only.

## Pre-flight (run before Phase A — report results immediately)

### Swagger drift check
Run `make docs` and then `git diff --exit-code docs/swagger/ frontend/src/api/types.ts`.
- Non-empty diff = **blocker**. List the files that changed and tell the author to commit them.
- This is the authoritative swagger check — agents do not need to repeat it.

### Codecov annotations (if a PR exists and CI has run)
```bash
REPO=$(gh repo view --json nameWithOwner -q .nameWithOwner)
gh pr checks <PR> --json name,conclusion,detailsUrl | jq '.[] | select(.name | test("codecov"; "i"))'
gh api repos/${REPO}/issues/<PR>/comments \
  --jq '[.[] | select(.user.login == "codecov[bot]")] | last | .body' 2>/dev/null
```
Extract:
- **Patch coverage** ≥ 40% (informational)
- **Project coverage** delta — must not drop > 2% (blocking if CI fails on it)
- **File-level table** — flag files where coverage dropped > 5 percentage points as should-fix
- If no Codecov comment yet: note "pending" and skip

### Commit structure audit
Run `git log --oneline origin/main..HEAD`. Check two things:
1. **Bare fixup! commits**: any commit whose subject starts with `fixup!` is a **blocker** — the author must run `git rebase --autosquash origin/main` and force-push before merge.
2. **Bundled concerns**: a commit that addresses unrelated changes (e.g. a bug fix + a refactor + a new feature) is a **blocker** — the branch must be split.

### Test coverage audit
Read the PR diff and test plan, then for each new exported function/method/handler:
1. Does a test exist for the happy path? Required.
2. Does a test exist for the primary error path? Required.
3. For handlers: does a test exist for bad input (400)? Required.
4. Are there edge cases handled in the code (nil guard, empty slice, zero count) with no corresponding test? Flag as gap.
5. For each test plan item that describes automatable behaviour ("endpoint X returns Y", "function handles Z"), is there a test in the diff? If not, **blocker**.

Report coverage gaps here, before agents launch, so the user sees them immediately.

## Code change detection (do this before Phase A)

Run `git diff --name-only origin/main...HEAD` and check whether any changed file has a code extension (`.go`, `.ts`, `.tsx`, `.sql`, `.py`, `.sh`).

- If **yes**: launch both Phase A agents (code-quality-reviewer + integration-tester).
- If **no** (docs, markdown, YAML, skill files only): launch **only the code-quality-reviewer**. Skip the integration-tester entirely and note "N/A (docs-only)" in the final report under Live API smoke test.

## Phase A — Parallel reviews

Launch agents in a **single message** (parallel tool calls). Wait for all to return before Phase B.

### Agent 1: code-quality-reviewer

Prompt must include:
- Base branch, PR number, file list from `git diff --stat`
- PR intent (from title/body, one sentence)
- The project conventions block below — paste it verbatim
- Instruction to focus on the diff only, write the report as the final message, do not commit anything

**Full-stack path tracing — include this verbatim in the prompt:**

```
FULL-STACK PATH VERIFICATION

For every new feature in this PR, trace the complete path from frontend to database and
verify each layer agrees with the next. Do not assume a layer is correct because it compiles.

1. FRONTEND → BACKEND CONTRACT
   - What URL and HTTP method does the hook call? (check api.GET/POST path)
   - What query params or request body does it send? (check the hook's params object)
   - What response shape does it expect? (check how the hook reads data fields)
   - Does this match: the route registered in internal/api/routes.go, the handler's param
     parsing, and the response struct the handler encodes?
   - Are field names consistent end-to-end? A hook reading data.host_count when the handler
     returns HostCount (PascalCase, no json tag) is a silent bug.

2. BACKEND → SERVICE CONTRACT
   - Does the handler pass all the inputs the service method needs?
   - Does the handler correctly interpret the service's return values? (e.g. uuid.Nil meaning
     "not queued" vs an error meaning "failed")
   - Are error types mapped to the right HTTP status codes? (CodeNotFound → 404,
     CodeUnknown → 500, scanning.ErrQueueFull → 429)

3. SERVICE → DATABASE CONTRACT
   - Does the SQL query reference columns and tables that exist in the schema?
   - Does the query return columns in the order the scan destination expects?
   - Are nullable columns handled? (sql.NullString, sql.NullTime — not bare string/time.Time)
   - Does the service correctly handle sql.ErrNoRows vs other DB errors?
   - If the service filters or aggregates results, does the filter logic match the intent?
     (e.g. filtering out null os_family rows when the SQL JOIN returns them)

4. SCHEMA → MIGRATIONS
   - Does any new code assume a column or index that isn't in the migration files?
   - If a migration adds a NOT NULL column to an existing table, is there a default or backfill?

Flag any mismatch at any layer as a blocker — these are the bugs that pass all unit tests
and only surface at runtime.

5. UNEXPOSED API SURFACE
   After tracing all new endpoints, check whether any backend capability added in this PR
   has no corresponding frontend hook or UI. This is not a blocker, but flag it as a
   "nice to have" with a concrete suggestion: which hook file it should go in, what the
   query key should be, and which page or component would benefit from surfacing it.
   Examples: a new filter param on an existing endpoint that the frontend ignores, a new
   aggregation endpoint with no widget, a new action endpoint with no button.
```

**Logical correctness — include this verbatim in the prompt:**

```
LOGICAL CORRECTNESS (check these before style)

Read each new function and ask:
- Does the logic actually do what the name and comment claim? Look for off-by-one errors,
  wrong comparison operators, inverted conditions, and missed early returns.
- Are there implicit assumptions that could be violated? (e.g. assumes slice is non-empty,
  assumes map key exists, assumes context is non-nil, assumes DB returns rows in a specific order)
- Is every code path handled? Trace through: what happens if the DB returns 0 rows? What if
  the input is empty string? What if a pointer is nil?
- Are function signatures complete and consistent with how callers use them? Look for:
  - Parameters that are passed but ignored inside the function
  - Return values that callers never check
  - Context not threaded through when the function does IO
  - Methods that modify receiver state without the receiver being a pointer
- API contract mismatches: does the handler's response shape match what swagger_docs.go declares?
  Does the frontend hook's expected response shape match what the backend actually returns?
  Does the query param name in the hook match the param name the handler reads?
- DB schema assumptions: does new code reference columns or tables that exist in the migrations?
  Does it assume a column is NOT NULL when the schema allows NULL? Does it use the right type
  (uuid vs string, timestamptz vs timestamp)?
- Interface completeness: if a new method is added to an interface, are all implementations
  updated? Check mock structs in test files — a mock that doesn't implement a new method will
  fail to compile but may be in a file the author didn't update.
- Concurrency: are shared fields accessed under a lock? Are goroutines properly waited on?
  Does a goroutine capture a loop variable by reference?
```

**Project conventions to enforce — include this verbatim in the prompt:**

```
Project-specific conventions to enforce (from CLAUDE.md):

COMMIT STRUCTURE
- Each commit must address exactly one concern. Flag any commit that bundles unrelated changes.

GO CONVENTIONS
- Handlers: tested via httptest + gorilla/mux with a local mock struct using function fields.
  Every new handler needs tests for: 200 success, 400 bad input, primary error path (404/500).
- JSON key assertion: use map[string]any to decode responses, not the Go struct — this catches
  missing json:"..." tags. Assert snake_case keys; flag any PascalCase keys in JSON output.
- Nil vs empty slice: methods that return slices to be JSON-encoded must return make([]T, 0),
  never nil. Test that the wire format is [] not null.
- Error types: use apierrors.NewScanError(apierrors.CodeNotFound, ...) for typed errors,
  fmt.Errorf("context: %w", err) for wrapping.
- Test assertions: require for fatal (stops test on failure), assert for non-fatal. Never use
  require where the test can meaningfully continue.
- Services: DB-touching code uses go-sqlmock. Verify ExpectationsWereMet() is called at the
  end of every test that sets up mock expectations.
- Routes: if a new handler is added, verify it is registered in internal/api/routes.go.
  A handler that isn't registered compiles and tests fine but doesn't exist at runtime.

TYPESCRIPT CONVENTIONS
- Every hook used in a page component must have a vi.mock(...) at the top of the component's
  test file and a default return in setupDefaultMocks(). Missing mock = "No QueryClient set" in CI.
- Hook tests: use renderHookWithQuery. Provide ok() and fail() helpers. Cover loading state,
  success with real field assertions, and error state.
- Component tests: assert on rendered text, not internals. Test loading skeleton (animate-pulse),
  empty state, and error state in addition to happy path.
- Tautological tests (asserting language invariants or restating constants without exercising
  real logic) do not count as coverage.
```

For re-reviews: also include the previous review's blocker/should-fix list and instruct the agent to confirm each prior item is resolved before looking for new issues.

### Agent 2: integration-tester

Prompt must include:
- Base branch, PR number, file list, which subsystems the diff touches
- Instruction NOT to run `make dev-nuke` or destructive cleanup
- Use the existing dev environment if running; ask before starting one

**Cleanup requirement — include this verbatim:**
> Before starting, record the current counts of scan jobs, profiles, and any other mutable resources the tests will touch (e.g. `SELECT status, COUNT(*) FROM scan_jobs GROUP BY status`). After all tests complete, delete every resource the test session created. Report before/after counts to confirm cleanup. If a DELETE endpoint doesn't exist, delete directly via the database. Leaving test artifacts is a bug in the test run.

**Scan failure reporting — include this verbatim:**
> If scans fail, report the specific error message from the `error_message` column in `scan_jobs`, not just "expected without sudo". Distinguish: (a) permission failures (raw socket / sudo — acceptable in dev), (b) queue-full failures (test design issue — flag it), (c) any other error. Never describe a failure as "expected" without quoting the actual error.

**What the integration-tester should check:**
- Live API responses match the expected shape (status codes, response body fields)
- New routes are reachable (catches missing routes.go registration)
- Any cross-layer contract: does the frontend hook's query key and params match what the backend expects?
- Note explicitly which frontend behaviours cannot be verified without a browser (empty state rendering, loading skeleton) so the gap is visible in the report

For re-reviews: skip re-testing subsystems that passed in round 1 and have no new commits. Focus on previously-flagged blockers and changed subsystems only.

## Phase B — Consolidation

Once both reports are back, launch the third agent **sequentially**:

**Agent 3: pr-quality-reviewer**

Include:
- Branch + base + PR number
- Full reports from both agents, clearly labeled
- Current CI status from `gh pr view --json statusCheckRollup`
- Pre-flight findings (swagger, codecov, commit audit, coverage gaps)
- Instruction to deduplicate findings, resolve conflicts explicitly (not silently), and produce a single prioritized merge-or-block decision

## Phase C — Final report

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
- Routes registration: <pass/fail/n/a>
- Codecov patch coverage: <X% / pending / no PR>

### Recommendation
<merge | merge after fixing blockers | block — reason>
```

Offer to print any agent's full report on request.

## Rules

- **Never commit, push, force-push, or merge anything** as part of this skill. Read-only.
- **Don't modify config files** or run `make dev-nuke`.
- If the dev environment isn't running and the user is on a feature branch, ask before running `make dev` (it requires sudo).
- If agents contradict each other, surface the contradiction explicitly — never resolve it silently.
- If there's no PR yet, still run the review — note "no PR yet" in CI status.
- **Swagger drift is checked in pre-flight only** — agents do not need to re-run `make docs`.
- **Do not re-verify in round 2+ what was already clean in round 1 and hasn't changed.**
