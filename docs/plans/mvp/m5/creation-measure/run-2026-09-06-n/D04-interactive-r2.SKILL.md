---
name: server-log-error-escalation
description: Use when you need to process a server log, count ERROR lines, and decide whether to create a Jira issue, notify the on-call engineer, and produce a daily summary.
---

# Skill

Use this skill when you are given a server log and need to count `ERROR` lines, check whether the count is over 10, and then carry out the confirmed reporting sequence.

Two rules apply:
- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write `not given` where it is silent;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Steps

1. 讀取伺服器 log 檔
   - Read the log text exactly as provided.
   - If the log text is missing, say `not given` for the log content and continue only with what is present.

2. 篩出 ERROR 行
   - Count the lines that contain `ERROR`.
   - Record the error count as a number.

3. 錯誤是否超過 10 筆
   - Compare the error count to 10.
   - Set `over_10` to `yes` when the count is greater than 10; otherwise set it to `no`.

4. 建立 Jira 問題單
   - If `over_10` is `yes`, set `create_jira` to `yes`.
   - If `over_10` is `no`, set `create_jira` to `no`.

5. 通知值班工程師
   - If `over_10` is `yes`, set `notify_on_call` to `yes`.
   - If `over_10` is `no`, set `notify_on_call` to `no`.

6. 寫入每日摘要
   - Always set `write_daily_summary` to `yes`.
   - Write the summary as a short final sentence based on the log and the decision.

## Output

Return exactly these five labeled lines, in this order:
- `error_count: <number>`
- `over_10: yes|no`
- `create_jira: yes|no`
- `notify_on_call: yes|no`
- `write_daily_summary: yes`
- `daily_summary: <one short sentence>`

Do not add extra sections or unlabeled text.