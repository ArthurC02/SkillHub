---
name: server-log-error-triage-skill
description: Use this skill when you need to turn a server log into a simple error-triage workflow: count ERROR lines, check whether they exceed 10, and follow the matching branch for Jira creation, on-call notification, and daily summary writing.
---

# Server Log Error Triage

## Purpose
Follow the confirmed flow exactly:
1. Read the server log file.
2. Filter out ERROR lines.
3. Check whether the error count is greater than 10.
4. If yes, create a Jira issue, notify the on-call engineer, then write the daily summary.
5. If no, write the daily summary.

## How to run
Given input that contains readable server log content, do the following in order:

1. **讀取伺服器 log 檔**  
   Read the provided server log content. If the input does not contain readable log text, stop and report that the log content is missing.

2. **篩出 ERROR 行**  
   Identify every line marked `ERROR`. Count only the lines that are explicitly ERROR lines in the supplied log text.

3. **錯誤是否超過 10 筆**  
   Compare the ERROR-line count to 10.

4. **建立 Jira 問題單**  
   If the count is greater than 10, create a Jira issue. The skill does not specify the Jira fields, project, or ticket format because the confirmed flow does not provide them.

5. **通知值班工程師**  
   If the count is greater than 10, notify the on-call engineer. The skill does not specify the notification channel or message format because the confirmed flow does not provide them.

6. **寫入每日摘要**  
   Write the daily summary in both branches. The skill does not specify summary content or format because the confirmed flow does not provide them.

## Branching rule
- If ERROR count is **greater than 10**: follow the yes branch in order: create Jira issue, notify the on-call engineer, then write the daily summary.
- If ERROR count is **10 or less**: follow the no branch and write the daily summary directly.

## Guardrails
- Do not add steps beyond the confirmed nodes.
- Do not invent extra conditions, alternate branches, or additional outputs.
- Do not assume a specific log source, log schema, Jira configuration, or notification tool.
- When the input is sufficient to count ERROR lines, complete the flow in one pass without asking follow-up questions.
- When the input does not contain readable log content, state that the log content is missing.