---
name: github-issue-severity-triage
description: Classify GitHub issues into three severity levels and provide one handling suggestion per level. Use this skill when you need a quick, consistent triage summary for multiple issues or a single issue set that must be prioritized before action.
---

# GitHub Issue Severity Triage

## Purpose
Use this skill when you need to review one or more GitHub issues, assign each issue to one of three severity levels, and produce one concise handling suggestion for each level.

## Output model
Always produce:
1. A per-issue severity label.
2. A brief reason for the label.
3. One handling suggestion for each of the three severity levels.

Use exactly three severity levels:
- **High**: urgent, user-facing, blocking, security-related, data-loss-related, or causing major functional failure.
- **Medium**: important but not immediately blocking; degraded behavior, performance issues, partial workarounds, or limited impact.
- **Low**: minor, cosmetic, informational, or low-impact issues.

## How to triage
1. Read each issue independently first.
2. Determine impact, scope, urgency, and whether there is a workaround.
3. Assign the issue to the highest severity that is justified by the evidence in the issue text.
4. If the issue text is insufficient to decide, label it as **Unclear** and state what information is missing.
5. Keep the mapping from issue text to severity explicit so the result is traceable.

## Writing the result
For each issue:
- Repeat the issue identifier or a short quoted summary.
- Assign one severity label: High, Medium, Low, or Unclear.
- Give one short reason grounded in the input.

Then provide a short recommendation for each severity level:
- High: suggest immediate investigation or mitigation.
- Medium: suggest scheduling soon and confirming scope or workaround.
- Low: suggest batching with routine maintenance or backlog cleanup.

## Constraints
- Do not invent facts not present in the issue text.
- Do not merge separate issues into one severity decision.
- Do not produce more or fewer than three severity-level recommendations.
- If multiple issues share the same level, they may share the same recommendation, but each issue still needs its own label and reason.

## Suggested response structure
- Issue 1: [severity] — [reason]
- Issue 2: [severity] — [reason]
- Issue 3: [severity] — [reason]

Recommendations:
- High: [one sentence]
- Medium: [one sentence]
- Low: [one sentence]

If any issue is unclear:
- Unclear: [missing information needed to classify]
