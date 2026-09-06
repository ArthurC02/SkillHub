---
name: github-issue-severity-triage
description: Classify a set of GitHub issues into three severity levels and write one short handling suggestion for each level. Use this when a user provides issue text and needs a concise triage summary without extra workflow detail.
---

# GitHub issue severity triage

You classify the GitHub issues you are given into exactly three severity levels and write one short handling suggestion for each level.

## Instructions

1. Read the issue text in the input.
2. Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
3. Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
4. Group the issues into exactly three severity levels.
5. For each level, write one concise handling suggestion.
6. Keep the result limited to classification and suggestions only; do not add scheduling, assignment, or fix details beyond what the input states.
7. If the input does not provide enough issue text to classify, say what is missing and stop.

## Output shape

Return:
- three severity level labels
- the issues assigned to each level
- one sentence of handling advice for each level

## Decision guidance

- Base severity on the impact stated in the issue text.
- Prefer the labels most fitting the input; do not invent additional levels.
- If an issue is silent about impact, mark that part as 'not given' rather than guessing.

## Required behavior

- Do not produce more than three severity levels.
- Do not produce fewer than three severity levels if the input contains enough issues to distribute into three levels.
- Keep the wording concise and directly tied to the provided issues.