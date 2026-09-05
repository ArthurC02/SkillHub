---
name: server-log-error-triage
description: Detects ERROR lines in server logs and routes follow-up actions based on whether the error count exceeds 10; use this when you need a portable skill to triage logs into either Jira-plus-oncall escalation or a daily summary only.
---

# Server Log Error Triage

## Purpose
Use this skill when you need to inspect one or more server log inputs, count `ERROR` lines, and decide whether to escalate or only summarize.

## Inputs
- One or more server log files, or pasted log text.
- No assumptions are made about log format beyond the presence of lines that contain `ERROR`.

## Procedure
1. Read the supplied log content.
2. Identify lines that contain `ERROR`.
3. Count the matching `ERROR` lines.
4. Compare the count to the threshold of 10.
5. If the count is greater than 10:
   - Create a Jira issue.
   - Notify the on-call engineer.
   - Write the daily summary.
6. If the count is 10 or fewer:
   - Skip Jira issue creation.
   - Skip on-call notification.
   - Write the daily summary.

## Output requirements
- State how many `ERROR` lines were found.
- State whether the count is greater than 10.
- State which branch was taken.
- Always include the daily summary step.
- Do not invent Jira fields, summary format, or notification channel details when they are not provided.

## Constraints
- Preserve unspecified implementation details as unknowns.
- Do not assume whether logs come from one server or multiple servers.
- Do not add extra actions beyond the confirmed flow.

## Acceptance check guide
A correct run of this skill must make the threshold decision explicit, show the `ERROR` count, and include the required downstream actions for the selected branch.