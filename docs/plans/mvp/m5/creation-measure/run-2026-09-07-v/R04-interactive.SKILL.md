---
name: github-issue-severity-triage
description: Classify GitHub issues into three severity levels and add one concise handling suggestion per level when you need a reusable triage format for issue lists.
---

# GitHub Issue Severity Triage

Use this skill when you are given GitHub issue text and need to sort each issue into one of three severity levels with one concise handling suggestion for each level.

Follow these instructions exactly:

1. Read the issue list in the input.
2. Classify each issue into one of exactly three severity levels.
3. Give one sentence of handling advice for each severity level.
4. Keep each issue paired with its classification and advice.
5. Present the result in a clear, readable structure.

Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.

Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Output requirements

- Use exactly three severity levels.
- Assign every issue to one of those three levels.
- Provide one handling suggestion for each severity level.
- Preserve the association between each issue and its assigned level.
- If the input does not provide enough information to classify an issue, write 'not given' for the missing detail rather than inventing it.

## Suggested structure

- Severity level name
- One-sentence handling suggestion
- Issues assigned to that level

Repeat until all issues are covered.

## Final check

Before responding, verify that every input issue appears in the output, every issue has exactly one severity classification, and each severity level includes one sentence of advice.