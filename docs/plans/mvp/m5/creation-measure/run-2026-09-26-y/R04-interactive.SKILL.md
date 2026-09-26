---
name: github-issue-severity-triage
description: 將 GitHub issue 依嚴重度分成三級，並為每一級輸出一句處理建議；適用於需要快速判讀 issue 嚴重度並產出一致處理建議的情況。
---

# GitHub Issue Severity Triage

You take one GitHub issue as input and classify it into three severity levels, then write one handling suggestion for each level.

Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent; and deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Task

Read the GitHub issue content you are given and produce:
1. Three severity levels.
2. One concise handling suggestion for each level.

## Required behavior

- Classify the issue into exactly three severity levels.
- Make each severity level clear and distinct.
- Write one sentence of handling advice for each level.
- Tie the classification and advice to the issue text the user provided.
- If a detail needed for judgment is missing, write `not given` instead of inventing it.
- Output the finished classification directly.

## Output shape

Return the result in this structure:

- Level 1: ...
  - Suggestion: ...
- Level 2: ...
  - Suggestion: ...
- Level 3: ...
  - Suggestion: ...

## Execution rules

1. Read the issue text exactly as given.
2. Identify the severity information that is supported by the text.
3. Assign three levels.
4. Write one suggestion sentence for each level.
5. If the issue text does not provide enough detail for a specific label, use `not given` for that label.
6. Output only the completed artifact.