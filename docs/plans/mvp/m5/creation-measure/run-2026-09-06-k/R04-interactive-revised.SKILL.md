---
name: github-issue-severity-triage
description: 將 GitHub issue 依嚴重度分成三級，並為每級提供一句處理建議；在需要快速整理 issue 優先順序與回覆建議時使用。
---

# GitHub Issue Severities Triaging

## Purpose
Read GitHub issue text, classify the issues into three severity levels, and provide one concise handling suggestion for each level.

## When to use
Use this skill when you need a fast, text-only summary of GitHub issues by severity, along with a practical next-step suggestion for each severity tier.

## Instructions
1. Read the issue text provided by the user.
2. Identify the issues described in the text.
3. Group them into exactly three severity levels.
4. For each severity level, write exactly one sentence of handling advice in Chinese.
5. Keep the output grounded only in the user-provided text.
6. Do not invent missing background, impact, owners, timelines, or technical causes.
7. Do not include content unrelated to the three severity levels and their advice.
8. Present the result in a clear structure that pairs each severity level with its advice.

## Output shape
- Use three clear sections, one for each severity level.
- In each section, include:
  - the severity level name
  - the issues assigned to that level, if any are present in the input
  - one Chinese sentence of handling advice

## Constraints
- Output must be in Chinese.
- Use exactly three severity levels.
- One advice sentence per level.
- Base the classification only on the input text.
- If the input does not contain enough information to separate issues clearly, still provide the best three-level grouping grounded in the text and do not ask follow-up questions.
- Do not infer technical root causes or operational details that are not explicitly present in the input.
- Keep advice limited to actions supported by the described issue text.
