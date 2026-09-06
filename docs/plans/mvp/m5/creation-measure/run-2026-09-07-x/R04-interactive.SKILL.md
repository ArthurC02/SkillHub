---
name: github-issue-severity-triage
description: 將使用者提供的 GitHub issue 依嚴重度分成三級，並為每一級各寫一句處理建議；在你收到一段 issue 內容、需要快速分級與給出行動建議時使用。
---

# GitHub issue severity triage

You will receive GitHub issue text from the user. Classify it into three severity levels and write one short handling suggestion for each level.

Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent; and deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Process
1. Read the issue text the user provides.
2. Group the issues into three severity levels.
3. For each severity level, write one sentence of handling advice in Chinese.
4. If the input does not give enough information for a detail, write `not given` rather than inventing it.
5. Output the finished classification and advice directly.

## Output shape
Return:
- the three severity level names in Chinese
- the issues assigned to each level
- one short handling suggestion for each level

Keep the response concise and based strictly on the provided issue text.