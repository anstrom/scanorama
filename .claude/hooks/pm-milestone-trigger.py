#!/usr/bin/env python3
"""UserPromptSubmit: when the user says a milestone is done, point Claude at the product-manager skill.

Feature-suggestion phrasing is left to the skill's own description; matching it here fired on
ordinary implementation requests.
"""
import json
import re
import sys

PATTERNS = [
    r"\bmilestone\b.*\b(?:is|are|now|all)\b.*\b(?:complete|done|finished|closed|shipped)\b",
    r"\b(?:closed|finished|wrapped up|done with|shipped)\b.*\bmilestone\b",
    r"\b(?:released|shipped|tagged)\s+v\d+\.\d+",
    r"\bv\d+\.\d+(?:\.\d+)?\s+(?:is\s+)?(?:out|released|shipped)\b",
]

try:
    prompt = (json.load(sys.stdin).get("prompt") or "").lower()
except (ValueError, AttributeError):
    sys.exit(0)
if any(re.search(p, prompt) for p in PATTERNS):
    print(json.dumps({"hookSpecificOutput": {
        "hookEventName": "UserPromptSubmit",
        "additionalContext": "The user message indicates a milestone may be complete. Use the "
        "product-manager skill to review milestone status, update the roadmap, and plan the next milestone.",
    }}))
