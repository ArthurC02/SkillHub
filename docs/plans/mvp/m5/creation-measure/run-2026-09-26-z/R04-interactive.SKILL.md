---
name: github-issue-severity-triage
description: 將使用者提供的 GitHub issue 依嚴重度分成三級，並為每一級各寫一句處理建議；當需要快速整理 issue 優先順序與修復建議時使用。
---

# GitHub Issue Severity Triage

You receive GitHub issue content from the user. Classify the issues into three severity levels and write one sentence of handling advice for each level.

## Instructions

1. Read the issue content the user provides.
2. Group the issues into three severity levels.
3. For each severity level, write exactly one sentence of handling advice.
4. Output in Chinese.
5. Make sure the classification and advice are grounded in the issue content the user provided.
6. If the user does not provide enough issue content to classify, say what is missing and stop.

## Required rules

- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Output shape

Provide three severity levels, each with:
- the severity label
- the issues assigned to that level
- one sentence of handling advice

## Notes

- Use the user’s wording where possible.
- Do not invent issue details that are not given.
- Keep the response concise and directly usable.