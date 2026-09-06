---
name: server-log-error-escalation
description: Use this skill when you need to turn a server log review into a simple escalation workflow: count ERROR lines, decide whether the count exceeds 10, and then produce the corresponding Jira, on-call, and daily-summary actions.
---

# Server Log Error Escalation

## Purpose
Use this skill when you are given a server log and need to apply a fixed escalation rule: extract `ERROR` lines, count them, and decide whether the count is greater than 10.

## Inputs
- One server log text.

## Process
1. Read the server log text.
2. Identify the lines that count as `ERROR` lines.
3. Count the `ERROR` lines.
4. Check whether the count is more than 10.
5. Follow the matching branch below.

## Branches
### If the number of `ERROR` lines is greater than 10
- Create a Jira issue.
- Notify the on-call engineer.
- Write the daily summary.

### If the number of `ERROR` lines is 10 or fewer
- Write the daily summary.

## Outputs
Return a concise result that states:
- the number of `ERROR` lines found,
- whether the threshold was exceeded,
- which branch was taken,
- whether a Jira issue was created,
- whether the on-call engineer was notified,
- whether the daily summary was written.

## Tool Requirements
- No external tools are assumed by default.
- If a Jira or notification tool is available in the runtime, use it only when the `ERROR` count is greater than 10.
- If no such tools are available, report that the branch calls for Jira creation and on-call notification, but do not invent tool behavior.

## Limitations
- Do not add steps that are not part of the confirmed flow.
- Do not assume a specific log source, log file format, notification channel, or daily-summary destination.
- Do not redefine the `ERROR` filter beyond the confirmed requirement to look for `ERROR` lines.
- Keep the workflow to the confirmed sequence: read log, filter `ERROR`, evaluate the threshold, then follow the branch.

## License
Apache-2.0
