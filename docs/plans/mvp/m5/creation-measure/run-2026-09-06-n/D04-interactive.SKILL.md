---
name: server-log-error-escalation
description: Use when you need to process a server log, count ERROR lines, and decide whether to create a Jira issue, notify the on-call engineer, and produce a daily summary.
---

# Skill

Use this skill when you are given a server log and need to count `ERROR` lines, check whether the count is over 10, and then carry out the confirmed reporting sequence.

Two rules apply:
- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Steps

1. 讀取伺服器 log 檔
   - Read the log text exactly as provided.
   - If the log text is missing, refuse to continue and say the input is not given.

2. 篩出 ERROR 行
   - Count the lines that contain `ERROR`.
   - Record the error count.

3. 錯誤是否超過 10 筆
   - Compare the error count to 10.
   - If the count is over 10, follow the confirmed `是` branch.
   - If the count is 10 or fewer, follow the confirmed `否` branch.

4. 建立 Jira 問題單
   - When the count is over 10, produce the Jira issue creation result in the output.
   - If the input does not provide Jira issue details, write `not given` for those details.

5. 通知值班工程師
   - After Jira issue creation, produce the on-call notification result in the output.
   - If the input does not provide notification details, write `not given` for those details.

6. 寫入每日摘要
   - Finish by producing the daily summary in the output.
   - If the input does not provide summary details, write `not given` for those details.

## Output

Return a structured result that directly shows:
- error count
- whether it is over 10
- whether Jira was created
- whether the on-call engineer was notified
- the daily summary

If a field is not present in the input, write `not given`.