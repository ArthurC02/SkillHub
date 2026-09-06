---
name: github-issue-severity-triage
description: 將使用者提供的 GitHub issue 依嚴重度分成三級，並各寫一句處理建議；當需要把多個 issue 快速分類、整理優先順序並產出簡短處置建議時使用。
---

# GitHub issue severity triage

Use this Skill when the user gives GitHub issues and wants them sorted into three severity levels with one short handling suggestion for each level.

## Instructions

1. Read the GitHub issue text the user gives you.
2. Group each issue into one of three severity levels.
3. For each severity level, write exactly one sentence of handling advice.
4. Keep the original issue content visible in the output so the mapping from issue to severity and advice is clear.
5. Present the result in a format that makes the relationship **issue → severity → advice** easy to scan.
6. Use the three severity labels consistently.
7. If the input does not contain enough information to classify a specific issue, say 'not given' for the missing part.
8. Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
9. use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.

## Output shape

Provide a compact table or bullet list with these three columns or parts:
- issue
- severity level
- one-sentence handling advice

## Severity levels

Use exactly three levels. If the user has not specified names for the levels, choose simple labels and use them consistently throughout the output.

## Notes

- Do not rewrite the task as a policy or explanation.
- Do not omit any issue that appears in the input.
- Do not add extra categories beyond the three severity levels.
- Keep the advice to one sentence per level.