---
name: github-issue-severity-triage
description: Classify GitHub issues into three severity levels and write one brief handling recommendation for each level when you need a concise triage summary from issue text.
---

# Instructions

You will receive one or more GitHub issue texts. Classify them into exactly three severity levels and write one sentence of handling advice for each level.

Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.

Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## What to produce

Return:
1. Three severity levels.
2. The issues assigned to each level.
3. One handling recommendation sentence for each level.

## How to decide

- Base the classification only on the issue text provided.
- Do not use external information.
- Do not create more than three severity levels.
- Do not give more than one recommendation sentence per level.
- If a detail needed for triage is missing, say 'not given' instead of inventing it.

## Output shape

Use a clear, compact structure such as:
- Level name
- Issues in that level
- Recommendation: one sentence

Keep the result concise and directly tied to the input.