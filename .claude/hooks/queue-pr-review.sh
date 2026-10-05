#!/bin/sh
# PostToolUse(Bash): wake Claude to run /review-pr after a real `gh pr create`.
# Requires both a matching command and a PR URL in the tool output: settings.json's `if` filter
# passes complex commands (loops, heredocs) through, so the command alone would also fire on test
# scripts and commit messages that merely mention `gh pr create`. `gh pr edit` is deliberately
# excluded — body/label updates after a review would otherwise re-trigger the review.
jq -e '(.tool_input.command | test("(^|[;&|(\\n])\\s*([A-Za-z_][A-Za-z0-9_]*=\\S*\\s+)*gh pr create(\\s|$)"))
  and (.tool_response | tostring | test("github\\.com/[^/ ]+/[^/ ]+/pull/[0-9]+"))
  and (.tool_response | tostring | test("already exists") | not)' >/dev/null 2>&1 || exit 0
echo 'PR was just created. Your next action is to run /review-pr.' >&2
exit 2
