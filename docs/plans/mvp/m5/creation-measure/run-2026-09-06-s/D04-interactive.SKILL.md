---
name: server-log-error-triage
description: 從伺服器 log 文字中擷取 ERROR 行、計算筆數，並依是否超過 10 筆輸出對應處理結果；當你要把此流程做成可重複使用的 Agent Skill 時使用。
---

# Instructions

Use this skill when the user gives you server log text and wants ERROR lines extracted, counted, and triaged by whether the count exceeds 10.

Follow these steps in order, and do only what the input provides.

1. **讀取伺服器 log 檔**
   - Read the log text the user provided.
   - Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
   - Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
   - If the log text is missing, ask for it and stop.

2. **篩出 ERROR 行**
   - Extract every line that contains `ERROR`.
   - Keep the extracted lines exactly as given in the input.
   - If no ERROR lines are present, output `not given` for the ERROR-line list.

3. **建立 Jira 問題單**
   - Check whether the number of ERROR lines is greater than 10.
   - If it is greater than 10, create the Jira issue content from the input details only.
   - If the Jira issue details are not given in the input, write `not given` for the missing Jira content.

4. **通知值班工程師**
   - If the count is greater than 10, produce the notification content from the input details only.
   - If the notification recipient or message is not given, write `not given`.

5. **寫入每日摘要**
   - Always include the daily summary output.
   - If the count is greater than 10, include the summary after the Jira issue and notification outputs.
   - If the count is 10 or fewer, go directly to the daily summary.

## Output requirements
- Return a structured result with these fields, in this order:
  1. `ERROR_lines`
  2. `ERROR_count`
  3. `greater_than_10`
  4. `Jira_issue` (only when the count is greater than 10; otherwise `not given`)
  5. `on_call_notification` (only when the count is greater than 10; otherwise `not given`)
  6. `daily_summary`
- If any required value is missing from the input, write `not given`.
- Do not ask follow-up questions unless the input itself is missing the log text.
- Do not invent extra sections, dates, recipients, issue keys, or summaries.
