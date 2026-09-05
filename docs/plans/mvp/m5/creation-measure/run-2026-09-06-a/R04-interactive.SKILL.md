---
name: github-issue-severity-triage
description: Classify a GitHub issue into three severity levels and generate one short handling suggestion for each level when you need a quick, tool-agnostic triage result from issue text or a summary.
---

# GitHub Issue Severity Triage

## Purpose
Use this skill when you need to assess a GitHub issue from its text or summary, group it into three severity levels, and produce one short handling suggestion for each level.

## What this skill does
- Reads a single issue description, title, or summary.
- Classifies the issue into one of three severity levels: low, medium, or high.
- Produces one concise recommendation for how to handle each severity level.
- States missing information when the issue text is too vague to judge severity confidently.

## Operating rules
1. Work only from the issue text or summary provided by the user.
2. Do not assume access to GitHub, the repository, or issue metadata unless it is included in the input.
3. Do not claim to have queried GitHub APIs or any external system.
4. If the issue is incomplete, ask for the missing details needed to judge impact, scope, urgency, or reproducibility.
5. Keep the output short, structured, and suitable for human review or downstream automation.

## Severity model
Use these three levels consistently:

- **Low**: minor inconvenience, cosmetic problem, small edge case, or low-impact improvement.
- **Medium**: noticeable functional issue, partial workflow disruption, or problem affecting a subset of users or scenarios.
- **High**: severe failure, data loss risk, security concern, production outage, or broad user impact.

When deciding severity, consider:
- impact on users or systems
- how many people or workflows are affected
- whether there is a workaround
- urgency implied by the issue text
- whether the problem is blocking, degrading, or merely annoying

## Output format
Return exactly these three severity sections:

- **Low**: one sentence describing what makes the issue low severity and one short handling suggestion.
- **Medium**: one sentence describing what makes the issue medium severity and one short handling suggestion.
- **High**: one sentence describing what makes the issue high severity and one short handling suggestion.

If the input does not contain enough information to judge severity, add a brief **Missing information** section listing the specific details needed.

## Process
1. Read the issue text carefully.
2. Identify the user impact, scope, and urgency.
3. Map the issue to low, medium, or high severity.
4. Write one actionable sentence for each severity level.
5. If the input is too vague, explicitly name the missing details before finalizing the result.

## Style
- Use plain language.
- Keep each recommendation to one sentence.
- Avoid jargon unless it appears in the issue text.
- Do not over-explain the scoring logic.
- Make the result easy to scan.

## Example shape
- Low: ... Suggestion: ...
- Medium: ... Suggestion: ...
- High: ... Suggestion: ...

If details are missing:
- Missing information: ...
