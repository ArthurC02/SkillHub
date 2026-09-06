---
name: github-issue-severity-triage
description: Classify a GitHub issue into three severity levels and give one concise handling suggestion for each level when you need a structured severity summary from issue text.
---

# GitHub Issue Severity Triage

Use this skill when you need to turn one GitHub issue text into a three-level severity classification with one handling suggestion for each level.

## Inputs
- A GitHub issue text blob, which may include a title, body, comments, or a summary.
- Use only the text provided in the current input.

## Output
Produce Markdown with exactly these parts:
1. Three severity levels.
2. One concise handling suggestion for each level.
3. Clear section breaks so each level is easy to identify.

## Procedure
1. Read the issue text once and identify the strongest evidence about impact, scope, urgency, and production effect.
2. Assign the issue to three severity levels in a way that fits the provided text.
3. For each level, write one sentence that recommends what to do next.
4. Keep the wording grounded in the issue text; do not add background that is not present.
5. If the text is not enough to judge a point, state that uncertainty directly instead of guessing.

## Response shape
- Present the result in Markdown.
- Use one heading or label per severity level.
- Under each label, include exactly one sentence of handling advice.
- Do not add extra levels.
- Do not invent facts beyond the input text.