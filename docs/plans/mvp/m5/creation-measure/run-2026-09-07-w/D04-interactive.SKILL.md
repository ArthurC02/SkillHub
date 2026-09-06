---
name: server-log-error-triage
description: Use this skill when you need to scan a server log, count ERROR lines, and produce the corresponding triage outcome and daily summary.
---

# Server log error triage

Use this skill when you are given a server log and need to decide whether the number of ERROR lines is greater than 10, then produce the matching triage result and summary.

Follow the confirmed flow in order:

1. 讀取伺服器 log 檔
2. 篩出 ERROR 行
3. 建立 Jira 問題單
4. 通知值班工程師
5. 寫入每日摘要

## Instructions

- Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
- Read the server log exactly as provided in the input.
- Count the lines that are clearly ERROR entries.
- Determine whether the count is greater than 10.
- If the count is greater than 10, produce all of the following in the output:
  - a statement that the ERROR count is greater than 10
  - Jira issue creation
  - notification to the on-call engineer
  - the daily summary
- If the count is not greater than 10, produce only the daily summary outcome and do not include Jira issue creation or on-call notification.
- If the input does not include a server log, say 'not given'.
- If the input does not specify the output shape, use a concise Markdown result with clear sections for the count and actions.

## Output requirements

- State the ERROR count you found.
- State whether it is greater than 10.
- Include the required downstream actions only when the threshold is exceeded.
- Keep the result grounded in the supplied log text only.

## Notes

- Do not invent extra operational details.
- Do not ask follow-up questions if the log is present.
- Do not output a plan or explanation of how to perform the work.