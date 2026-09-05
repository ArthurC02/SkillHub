---
name: github-issue-severity-triage
description: Classify GitHub issues into three severity levels and give one concise handling suggestion for each level when you need a quick triage summary from issue text or issue summaries.
---

# GitHub Issue Severity Triage

## Purpose
Classify GitHub issues into three severity levels and provide one concise handling suggestion for each level.
Use this skill when you need a fast, text-only severity summary for one issue or a batch of issue summaries.

## Severity levels
Use exactly three levels:
- **High**: the issue blocks critical work, causes data loss, security risk, outage, or a major user-facing failure.
- **Medium**: the issue affects important functionality but has a workaround, limited scope, or partial impact.
- **Low**: the issue is minor, cosmetic, inconvenient, or has no meaningful user impact.

## Input handling
1. Read the issue title, body, labels, comments, or summary text that is available.
2. If only a title is provided and there is not enough context to judge impact, say the information is insufficient and ask for the issue description or more detail.
3. If multiple issues are provided, assess each issue separately and return one severity for each.
4. Do not assume hidden context. Base the classification only on the supplied text.

## Decision rules
1. Look for impact first: safety, data integrity, availability, security, or broad user disruption push the severity higher.
2. Look for scope next: many users or core workflows push higher than edge cases or rare conditions.
3. Look for workaround availability: if users can continue working, the severity is usually medium or low.
4. Look for urgency signals only as supporting evidence; do not rely on emotional wording alone.
5. When evidence is mixed, choose the lower severity unless the text clearly shows blocking or high-risk impact.

## Output format
Return a short, direct result in this shape:

- **Issue**: one-line issue identifier or summary
- **Severity**: High | Medium | Low
- **Reason**: one brief sentence explaining the classification
- **Suggestion**: one sentence describing the next handling step

If there are multiple issues, repeat the same structure for each one.

## Suggestion guidance
- **High**: recommend immediate investigation, escalation, or prioritization.
- **Medium**: recommend scheduling soon, confirming workaround, or assigning to the next available cycle.
- **Low**: recommend backlog, cleanup, or monitoring.

## Constraints
- Keep the output limited to severity classification and one handling suggestion per severity result.
- Do not create schedules, assign owners, update GitHub, or change issue content.
- Do not add unrelated workflow steps.
- If the input does not contain enough information to classify severity, ask for the missing issue details instead of guessing.