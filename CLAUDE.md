# Scanorama — Claude Code Rules

## Project State & Planning

**GitHub is the source of truth for current state and upcoming work.**

To determine the current version and what comes next:

```bash
git describe --tags --abbrev=0          # current released version
gh milestone list                       # active milestones = upcoming releases
gh issue list --state open --milestone <name>  # issues in the next milestone
```

- Local planning docs (`docs/planning/`) may be stale — always cross-check against GitHub milestones and open issues
- The next work items come from open issues on the active milestone, not from local docs
- When starting a new feature, check `gh issue view <NNN>` for the canonical spec

## Release

**Pushing a tag is the complete release action.** GoReleaser runs automatically via `.github/workflows/release.yml`.

```bash
git tag vX.Y.Z        # tag at HEAD of main
git push origin vX.Y.Z  # triggers CI → GoReleaser → GitHub release
```

Never use `gh release create`. Never write changelogs manually. The pipeline owns everything after the tag push.

## Branch Workflow

Always branch off `main`. Never commit directly to `main` (branch protection enforced).

```bash
git checkout main && git pull --rebase origin main
git checkout -b feat/my-feature
```

## Commits

### Format

```
<type>(<subsystem>): <description>
```

- **Scope** = subsystem name (`queue`, `handlers`, `smartscan`, `dashboard`) — never a plan/wave/ticket reference
- **Imperative mood**, present tense: "add X" not "added X"
- **No adjectives**, no marketing language: "implement pagination" not "add robust comprehensive pagination"
- **One concern per commit** — if a commit touches two unrelated things, split it

### Types

`feat` · `fix` · `docs` · `refactor` · `test` · `chore` · `ci` · `perf`

### One commit per logical concern

A PR should have as many commits as it has distinct concerns. Examples of correct granularity:

```
feat(db): add scan_results table migration
feat(api): add GET /scan-results endpoint
feat(dashboard): add scan results widget
ci: add nightly scan job
```

Do **not** squash these into one. Keep them discrete so the history is readable per subsystem.

### Fixup commits — for correcting mistakes only

If you catch an error in a previous commit on the same branch (lint failure, typo, missed edge case), fix it with a fixup rather than a new standalone commit:

```bash
git commit --fixup=<sha>             # ties the correction to the original
git rebase --autosquash origin/main  # folds it in before pushing
```

Never push bare `fixup!` commits. Never add a standalone "fix lint" or "oops" commit — always tie it back to the original with `--fixup`.

## Before Every Push

The git hooks enforce quality automatically:
- **pre-commit**: runs `gofmt`, `go vet`, and `golangci-lint` on every commit
- **pre-push**: runs backend tests (`-short -race`), frontend tests, and swagger drift check

Before pushing, squash fixup commits and verify the fuller test suite passes:

```bash
git rebase --autosquash origin/main
go test -race ./internal/...   # fuller than the hook's -short run (~30s)
git push origin my-branch      # hook runs lint + swagger + tests automatically
```

Run tests against the **committed** state, not the working tree. Unstaged changes can mask
failures CI will catch — ensure `git status` shows no unstaged modifications to source or
test files before running the suite.

If either test suite is red, fix before pushing — CI round-trip is 2–3 min.

## Pull Requests

- Every PR body must include `Closes #NNN` or `Fixes #NNN` (links to the issue and auto-closes it on merge)
- Use `Mentions #NNN` when the PR is related to but does not close the issue
- Merge strategy: always `gh pr merge --rebase` — never squash unless explicitly asked
- Update the PR title and body with `gh pr edit` after any significant change to the branch
- Run `review-pr` after every implementation — see **Code Review** section for how to prompt it

### Test plans

Test plans are for **live/manual verification only** — things CI cannot check.

Never list `go test`, lint, build steps, or swagger checks as manual checkboxes. If behaviour can be tested automatically, write the test — a checkbox is not a substitute.

## Linting

The pre-commit hook blocks commits with lint failures. To fix before committing:

```bash
make lint-fix   # auto-fixes formatting, imports, and simple issues
make lint       # shows what still needs manual attention
```

Key rules enforced by `.golangci.yml`:

| Rule | Constraint |
|------|-----------|
| `lll` | Lines ≤ 120 characters |
| `mnd` | No magic numbers in arguments, return values, conditions, cases, or operations |
| `nestif` | Extract deeply nested if-else blocks to helper functions |
| `goconst` | Strings appearing 3+ times must be named constants |
| `funlen` | Functions ≤ 100 lines and ≤ 50 statements |
| `gocyclo` | Cyclomatic complexity ≤ 15 |

Fix lint violations with `git commit --fixup` + autosquash — never standalone fix commits.

## Swagger

Run `make docs` after any handler, route, or type change. Swagger drift is a CI blocker.

```bash
make docs
git diff --exit-code docs/swagger/ frontend/src/api/types.ts  # must be empty
```

The pre-push hook catches this locally. A CI failure here means the hook was skipped.

## Testing

Tests are not optional. Every PR that adds or changes behaviour must include tests. "We'll add tests later" is not accepted.

### Backend (Go)

**Handlers** — test via `httptest` + `gorilla/mux`, never call the real service. Use a local mock struct with function fields so each test case wires only what it needs:

```go
type mockMyServicer struct {
    doThingFn func(ctx context.Context, id uuid.UUID) (*MyResult, error)
}
func (m *mockMyServicer) DoThing(ctx context.Context, id uuid.UUID) (*MyResult, error) {
    return m.doThingFn(ctx, id)
}
```

Every new handler needs tests for: **200 success**, **400 bad input**, **404/500 error path**.

Assert on JSON key names using `map[string]any` (not the Go struct) to catch missing `json:"..."` tags:
```go
var raw map[string]any
require.NoError(t, json.NewDecoder(w.Body).Decode(&raw))
assert.Equal(t, "expected", raw["snake_case_key"])
```

**Services** — use `go-sqlmock` for DB-touching code. Set up expectations with `mock.ExpectQuery(...).WillReturnRows(...)` and assert `mock.ExpectationsWereMet()` at the end. Use interface mocks (not real dependencies) for everything else.

**Nil vs empty slice** — when a method returns a slice that will be JSON-encoded, always return `make([]T, 0)` not `nil`. Test that the wire format is `[]` not `null`:
```go
assert.Equal(t, "[]\n", w.Body.String())
```

### Frontend (TypeScript)

**Hooks** — test via `renderHookWithQuery` from `test/utils`. Mock `api.GET/POST/PUT/DELETE` at the module level. Provide `ok()` and `fail()` helpers for clean response construction:

```ts
const ok = (data: unknown) => Promise.resolve({ data, error: undefined, response: new Response() })
const fail = (msg = "error") => Promise.resolve({ data: undefined, error: { message: msg }, response: new Response() })
```

Cover: loading state, success with real data assertions, error state.

**Page components** — test via `renderWithRouter`. Mock every hook the component uses with `vi.mock(...)` at the top of the file, and set defaults in `setupDefaultMocks()` called from `beforeEach`. Every hook in the component = one `vi.mock` in the test file, or CI fails with "No QueryClient set".

**What to assert** — check rendered text, not component internals. Test loading skeletons (`animate-pulse`), empty states, and error states in addition to the happy path.

### What counts as coverage

- Happy path: required
- Primary error path (service error, DB error, not found): required
- Bad input / validation (400): required for handlers
- Edge cases that appear in the PR (nil manager, empty result, zero count): required if the code handles them

Tautological tests that only assert Go/TS language invariants (e.g. `assert.Equal(t, "idle", "idle")`) do not count.

## Code Review

Run `review-pr` after every implementation, before reporting done or creating a PR. Never claim a task is complete without it.

Give the agent a targeted prompt — not just "review this". Include:

1. **What changed** — list the files modified and what each does
2. **What to check** — be specific based on what was implemented:
   - New handler → check: HTTP status codes correct, all error paths handled, JSON keys are snake_case, nil guard before encoding slice
   - New service method → check: error wrapping, context propagation, timeout, nil receiver guard
   - New DB query → check: sqlmock expectations match real query, `ExpectationsWereMet()` called, `sql.ErrNoRows` handled
   - New frontend hook → check: error state handled, loading state handled, staleTime set
   - New frontend component → check: all hooks mocked in test file, loading/empty/error states rendered
3. **Tests to verify** — point it at the test files and ask it to confirm coverage is non-tautological
4. **Project conventions to enforce** — snake_case JSON, `make([]T, 0)` not nil slices at API boundaries, `apierrors.NewScanError` for typed errors, `require` for fatal assertions and `assert` for non-fatal

Example prompt structure:
```
Review the implementation in these files: [list files]

Context: [one sentence on what the feature does]

Check specifically:
- [handler/service/hook-specific items]
- Test coverage: [list test files] — confirm happy path, error path, and bad input are covered
- Confirm no tautological tests
- Confirm JSON response keys are snake_case
- Confirm empty slices serialize as [] not null

Focus on the diff only. Report bugs and gaps, not style preferences.
```

## Go Code Conventions

- Return `make([]T, 0)` (not `var s []T`) when a nil slice would serialize as JSON `null` — prefer `[]` at API boundaries
- Wrap errors with `fmt.Errorf("context: %w", err)` for stack-traceable error chains
- Use `sql.ErrNoRows` specifically for not-found; wrap in `apierrors.NewScanError(apierrors.CodeNotFound, ...)`
- Repository pattern: all DB access via `*Repository` structs, never raw queries in handlers

## Project Structure

```
internal/
  api/handlers/   — HTTP handlers + tests
  api/routes.go   — route registration
  services/       — business logic
  db/             — repository pattern, migrations
  scanning/       — scan queue, job execution
  profiles/       — scan profile management
frontend/src/
  api/hooks/      — React Query hooks
  routes/         — page components
  components/     — shared UI
docs/
  swagger/        — generated (never edit manually)
  swagger_docs.go — source of truth for swagger annotations
```
