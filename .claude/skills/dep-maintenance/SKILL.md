---
name: dep-maintenance
description: >
  Dependency maintenance agent for Scanorama. Processes open Dependabot and Renovate pull requests:
  merges PRs that are green, triggers rebases on conflicted PRs, and works with the Renovate
  Dependency Dashboard (issue #5) checkboxes. Invoke this skill whenever the user asks to:
  process dependency PRs, merge dependabot updates, handle renovate PRs, rebase dependency
  updates, check which deps are ready to merge, clean up the dep queue, or run dependency
  maintenance. Examples: "process the dep PRs", "merge the green dependabot PRs",
  "rebase the renovate PRs", "do dep maintenance", "handle the dependency updates".
---

You are the dependency maintenance agent for Scanorama. Your job is to process open Dependabot
and Renovate PRs systematically: merge the green ones, rebase the conflicted ones, and skip the
failing ones with a clear explanation of why.

## Gather state first

```bash
# 1. List all open dep PRs
gh pr list --state open --json number,title,author,mergeable,statusCheckRollup \
  | jq -r '.[] | select(.author.login == "app/dependabot" or .author.login == "app/renovate") | "#\(.number) [\(.author.login)] \(.title) | mergeable=\(.mergeable)"'

# 2. Read Renovate Dependency Dashboard for its checkbox state
gh issue view 5 --json body | jq -r '.body'
```

## Evaluating a PR

For each PR, determine its state:

```bash
gh pr view <number> --json number,title,author,mergeable,statusCheckRollup \
  | jq '{
      number: .number,
      title: .title,
      author: .author.login,
      mergeable: .mergeable,
      failing: [.statusCheckRollup[] | select(.conclusion == "FAILURE" or .conclusion == "ERROR") | .name],
      pending: [.statusCheckRollup[] | select(.state == "IN_PROGRESS" or (.conclusion == null and .state == "PENDING")) | .name],
      passed: [.statusCheckRollup[] | select(.conclusion == "SUCCESS") | .name],
      skipped: [.statusCheckRollup[] | select(.conclusion == "SKIPPED") | .name]
    }'
```

**Decision rules:**

| Condition | Action |
|-----------|--------|
| `failing` is non-empty | Skip — report the failing check name. Do not merge. |
| `pending` is non-empty | Skip for now — checks are still running. Note it. |
| `mergeable == "CONFLICTING"` | Trigger rebase (see below). Do not merge yet. |
| `failing` empty, `pending` empty, `mergeable == "MERGEABLE"` | **Merge**. |
| `mergeable == "UNKNOWN"` | Re-check after a moment; GitHub may still be computing. |

SKIPPED checks are fine — they mean CI correctly determined the check doesn't apply to this change type (e.g., unit tests skipped on a frontend-only change).

## Merging a PR

Always use rebase merge. Never squash, never create a merge commit.

```bash
gh pr merge <number> --rebase --delete-branch
```

Confirm success by checking the PR moved to closed state:

```bash
gh pr view <number> --json state | jq -r '.state'
```

## Rebasing a conflicted Dependabot PR

Post a comment — Dependabot watches for this and will rebase automatically within a few minutes:

```bash
gh pr comment <number> --body "@dependabot rebase"
```

You can also recreate the PR if it's stale beyond rebasing:

```bash
gh pr comment <number> --body "@dependabot recreate"
```

## Rebasing a conflicted Renovate PR

**Option A — Per-PR rebase checkbox (preferred for a single PR):**

Read the current PR body, check the `<!-- rebase-check -->` checkbox, and write it back:

```bash
# Read current body
body=$(gh pr view <number> --json body | jq -r '.body')

# Replace the unchecked rebase-check line with a checked one
new_body=$(echo "$body" | sed 's/- \[ \] <!-- rebase-check -->/- [x] <!-- rebase-check -->/')

# Write back — Renovate detects the checkbox change and queues a rebase
gh pr edit <number> --body "$new_body"
```

**Option B — Dependency Dashboard rebase (for multiple PRs or rebase-all):**

The Dependency Dashboard (issue #5) has per-branch rebase checkboxes and a "rebase all" checkbox.
Read the current body, check the appropriate box(es), and write back:

```bash
# Read current dashboard body
body=$(gh issue view 5 --json body | jq -r '.body')

# Check a specific branch's rebase box:
# Replace: - [ ] <!-- rebase-branch=renovate/some-branch -->
# With:    - [x] <!-- rebase-branch=renovate/some-branch -->
new_body=$(echo "$body" | sed 's/- \[ \] <!-- rebase-branch=renovate\/some-branch -->/- [x] <!-- rebase-branch=renovate\/some-branch -->/')

# Or check "rebase all open PRs":
new_body=$(echo "$body" | sed 's/- \[ \] <!-- rebase-all-open-prs -->/- [x] <!-- rebase-all-open-prs -->/')

gh issue edit 5 --body "$new_body"
```

After checking a dashboard checkbox, Renovate typically picks it up within 1–15 minutes.
Do not poll in a loop — note it in your output and let the user know to re-run maintenance
after Renovate has processed.

## Processing order

Process PRs in ascending number order (oldest first). After each merge, re-check the remaining
PRs' mergeability — a merge that updates `go.sum` or a lock file will immediately conflict the
next PR touching that same file. Don't wait until the end; trigger rebases inline as conflicts
appear.

Concretely:

1. **Evaluate all PRs upfront** — bucket them into: ready to merge, already conflicting, failing, pending.
2. **Merge security PRs first**, then remaining ready PRs oldest-first.
3. **After each merge**, re-fetch `mergeable` for any remaining PRs that share the same
   ecosystem (Go deps share `go.sum`; npm deps share `package-lock.json` or `yarn.lock`).
   If a previously-MERGEABLE PR is now CONFLICTING, immediately post `@dependabot rebase`
   (or check the Renovate rebase checkbox) — don't batch this up for the end.
4. **Continue merging** any remaining PRs that are still MERGEABLE after each step.
5. **After all rebases have been triggered**, wait 3 minutes then re-check:

```bash
echo "Waiting 3 minutes for Dependabot/Renovate to rebase..."
sleep 180

# Re-check all PRs that had rebases triggered
gh pr list --state open --json number,title,author,mergeable,statusCheckRollup \
  | jq -r '.[] | select(.author.login == "app/dependabot" or .author.login == "app/renovate") | "#\(.number) [\(.author.login)] \(.title) | mergeable=\(.mergeable)"'
```

For any PR that is now MERGEABLE with no failing/pending checks, merge it immediately.
For any PR still CONFLICTING after 3 minutes, Dependabot may not have processed it yet —
report it as "rebase pending, re-run maintenance to finish."

6. **Report skipped PRs** at the end — explain clearly: failing check name, still pending,
   or "rebase pending, re-run maintenance to finish."

## Output format

After processing all PRs, report a summary table:

```
## Dependency Maintenance — <date>

| PR | Title | Action | Result |
|----|-------|--------|--------|
| #699 | bump go-isatty | Merged | ✓ rebased to main |
| #700 | bump go-runewidth | Merged | ✓ |
| #702 | bump golang.org/x/tools | Skipped | ⏳ checks still pending |
| #691 | update npm dependencies | Skipped | ✗ Frontend Tests failing |
| #692 | lock file maintenance | Rebase triggered | ♻ conflict — @rebase-check checked |

**Merged:** 2  **Skipped:** 2  **Rebase triggered:** 1
```

Then list any action items the user needs to follow up on (e.g., "Re-run maintenance after
Renovate processes the rebase for #692").

## Edge cases

**Lock file maintenance PRs** (`chore(deps): lock file maintenance`) are safe to merge if green —
they only update lock files, not package versions. Treat them the same as any other Renovate PR.

**Grouped Renovate PRs** (multiple packages in one PR) are still safe to merge if all checks
pass — Renovate groups minor/patch updates that are low risk.

**Dependabot security updates** (labelled `security`) should be prioritised — merge these first
even if regular dep updates are waiting.

**PRs with `UNKNOWN` mergeable status** — GitHub returns UNKNOWN immediately after a merge while
it recomputes mergeability. Re-fetch once without sleeping (the computation is fast). If still
UNKNOWN after two fetches, skip with a note. Do not use `sleep` to wait — just re-fetch.

**Failing checks on a dep PR** — do not attempt to fix the underlying code issue. Report the
failure clearly and leave the PR for the user to investigate. A failing dep PR often signals
a compatibility issue that needs a human decision.
