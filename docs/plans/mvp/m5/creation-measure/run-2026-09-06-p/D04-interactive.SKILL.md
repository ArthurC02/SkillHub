---
name: server-log-error-daily-flow
description: Use this skill when you need to turn a server log into a daily error-handling flow: count ERROR lines, decide whether the count is over 10, and produce the corresponding Jira / on-call / daily-summary outcome.
---

# Server Log Error Daily Flow

Follow the confirmed flow exactly and in order.

## Required instruction rules
- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## 1. 讀取伺服器 log 檔
Read the log text the user provides.
- If the log text is missing, stop and ask for the log text only.
- Use only the input the user supplied.
- If any detail needed for the result is silent, write `not given`.

## 2. 篩出 ERROR 行
Identify every line that is an ERROR line.
- Count the ERROR lines.
- Do not infer extra errors from context.
- Do not add lines that are not in the input.

## 3. 建立 Jira 問題單
Check whether the ERROR count is over 10.
- If it is over 10, include `建立 Jira 問題單` in the finished output.
- If it is not over 10, do not claim a Jira issue was created.
- Do not assume any Jira field values, issue keys, or connector behavior unless the input gives them; otherwise write `not given`.

## 4. 通知值班工程師
If the flow reaches this node, include `通知值班工程師` in the finished output only as supported by the input.
- Do not invent the notification channel, recipient, or automation details.
- If those details are not in the input, write `not given`.

## 5. 寫入每日摘要
Complete the result with the daily summary outcome.
- When the ERROR count is not over 10, include `寫入每日摘要`.
- Keep the output grounded in the input only.
- If the summary content is not given, write `not given`.

## Output
Deliver the finished artifact itself, not a plan or explanation.
Use a concise structured format that shows:
- ERROR count
- whether Jira was created
- whether the on-call engineer was notified
- daily summary outcome
- any missing details as `not given`

Do not add any step, condition, role, tool, or branch beyond the confirmed diagram.