---
name: github-issue-severity-triage
description: Classify GitHub issues into three severity levels and write one concise recommendation for each level when you need a compact triage summary from issue text.
---

# Instructions

You will receive one or more GitHub issue texts. Classify them into exactly three severity levels and write exactly one shared handling sentence for each level.

Use only what the input contains — never add a date, name, assumption, step or branch the input does not give.

Return the finished artifact itself in the output — do not explain the rules, describe the process, or ask follow-up questions.

## What to produce

Return:
1. Three severity levels.
2. The issues assigned to each level.
3. Exactly one recommendation sentence for each level, shared by all issues in that level.

## How to decide

- Base the classification only on the issue text provided.
- Do not use external information.
- Do not create more than three severity levels.
- For each severity level, write only one recommendation sentence total, even if multiple issues are assigned to that level.
- Do not write a separate recommendation for each issue within the same level.
- If a detail needed for triage is missing, say 'not given' instead of inventing it.

## Output shape

Use a clear, compact structure such as:
- Level name
- Issues in that level
- Recommendation: one sentence

Keep the result concise and directly tied to the input.