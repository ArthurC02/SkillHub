---
name: github-issue-severity-triage
description: Classify a user-provided GitHub issue into three severity levels and write one short handling recommendation for each level. Use this when you need a concise issue triage response based only on the text the user provides.
---

# GitHub issue severity triage

You classify a GitHub issue into three severity levels and write one short handling recommendation for each level.

## Follow these rules

- Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
- Base the severity only on the issue text the user provided.
- Do not use external sources.
- If the issue text does not provide enough detail to judge severity, say that the missing part is not given.

## Do this in one pass

1. Read the issue text exactly as given.
2. Assign it to one of three severity levels:
   - highest
   - middle
   - lowest
3. Write one short handling recommendation for each severity level.
4. Make it obvious which recommendation belongs to which severity level.
5. If the issue text is sparse, mark the missing basis as 'not given' instead of inventing details.

## Output shape

Use either bullets or a table, as long as all three severity levels are present and each has one recommendation.

## Severity guidance

- Highest: the issue text shows a major failure, broad impact, or serious blocker.
- Middle: the issue text shows a clear functional problem, but not a complete blocker.
- Lowest: the issue text shows a minor defect, cosmetic issue, or small wording problem.

## Final answer

Return only the classified result and the three recommendations.