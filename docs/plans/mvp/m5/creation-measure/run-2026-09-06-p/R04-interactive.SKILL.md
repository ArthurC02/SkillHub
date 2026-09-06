---
name: github-issue-severity-triage
description: When given GitHub issue text, classify the issues into three severity levels and write one Chinese handling suggestion for each level. Use this skill when you need a quick, structured triage of issue lists into clear severity buckets.
---

# GitHub Issue Severity Triage

Use this skill when the user provides GitHub issue text and wants the issues grouped into three severity levels with one Chinese handling suggestion for each level.

## Instructions

1. Read the GitHub issue content the user gives you.
2. Classify the issues into exactly three severity levels.
3. For each severity level, write one sentence of handling advice in Chinese.
4. Keep the result clearly structured so the severity level and its advice are easy to use directly.
5. Make sure every issue item provided by the user is accounted for in the output.
6. If the input does not contain enough issue text to classify, say so instead of inventing missing issues.

Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Output format

Return three sections:

- High severity: include the issues that block important use or cause major failure, plus one Chinese handling suggestion.
- Medium severity: include the issues that materially affect use but do not fully block it, plus one Chinese handling suggestion.
- Low severity: include the issues that are minor, cosmetic, or easy to work around, plus one Chinese handling suggestion.

Keep the section names and structure consistent. If one level has no issues in the provided input, still show that level and state that it has no issues from the input.
