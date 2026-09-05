---
name: github-issue-severity-triage
description: Classify a GitHub issue into three severity levels and write one concise handling suggestion for each level when you need a consistent triage summary from issue text.
---

# GitHub Issue Severity Triage

## Purpose
Classify a GitHub issue into three severity levels and provide one concise handling suggestion for each level.
Use this skill when you need a consistent, text-only triage summary from issue content.

## Input
- A GitHub issue description, title, or combined issue text.
- If multiple issue fields are available, read them together and base the result only on what is present.

## Output
Return exactly these parts:
1. A three-level severity classification.
2. One short handling suggestion for each severity level.
3. A brief note when the issue text is insufficient to judge severity confidently.

Use a stable, consistent format across runs.

## Severity model
Use three levels only:
- Low
- Medium
- High

Apply the levels by judging impact, urgency, and scope from the issue text:
- **Low**: limited impact, minor inconvenience, workaround likely exists, no immediate blocking effect.
- **Medium**: meaningful impact, repeated friction, partial blockage, or needs timely attention.
- **High**: severe impact, system outage, data loss risk, security concern, or major blocking effect.

## Procedure
1. Read the issue text carefully.
2. Identify explicit signals of impact, urgency, scope, blockage, data risk, or security risk.
3. Assign one severity level from the three-level model.
4. Write one concise handling suggestion for each level.
5. If the evidence is insufficient, say so clearly instead of guessing.
6. Do not add background facts, product context, or assumptions that are not in the issue text.

## Response format
Return markdown with this structure:

- **Severity**: Low | Medium | High | Unable to determine
- **Reasoning**: One short sentence grounded in the issue text.
- **Handling suggestions**:
  - Low: ...
  - Medium: ...
  - High: ...

If severity cannot be determined:
- **Severity**: Unable to determine
- **Reasoning**: State that the issue text does not provide enough evidence.
- **Handling suggestions**:
  - Low: Triage with the reporter for more details.
  - Medium: Triage with the reporter for more details.
  - High: Triage with the reporter for more details.

## Constraints
- Use only the information in the issue text.
- Do not infer missing context.
- Keep the output concise and consistent.
- Preserve the three severity levels; do not invent extra levels.
- If the input contains contradictory signals, favor the stronger impact signal and mention the ambiguity briefly.

## Quality checks
Before responding, verify that:
- Exactly three severity levels are represented.
- Each level has one sentence of handling advice.
- The answer does not rely on unprovided context.
- Any uncertainty is explicitly stated.