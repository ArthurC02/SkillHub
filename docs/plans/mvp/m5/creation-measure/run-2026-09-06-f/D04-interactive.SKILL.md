---
name: server-log-error-escalation
description: Use this skill when you need to turn a server log into an operational response: it filters ERROR lines, checks whether error count exceeds 10, and then decides whether to create a Jira issue, notify the on-call engineer, and write a daily summary.
---

# Server Log Error Escalation

Use this skill when the input is a server log and you need to follow the confirmed flow: read the log, extract ERROR lines, count them, and decide whether the count is above 10.

## Goal

Produce the operational output implied by the flowchart without inventing any missing Jira, notification, or summary details.

## Procedure

1. Read the entire server log content provided by the user.
2. Identify every line that contains `ERROR`.
3. Count the extracted `ERROR` lines.
4. Compare the count against the threshold of 10.
5. If the count is greater than 10:
   - create a Jira issue,
   - notify the on-call engineer,
   - write the daily summary.
6. If the count is 10 or fewer:
   - write the daily summary directly.

## Constraints from the confirmed diagram

- The Jira issue fields are not defined by the diagram. Do not invent issue contents, labels, workflow states, or templates.
- The notification method for the on-call engineer is not defined. Do not invent email, chat, paging, or any other delivery channel.
- The daily summary format and storage location are not defined. Do not invent a file format, destination, or naming convention.

## Required handling

- Treat the provided log as the only source of truth.
- Do not ask for extra details that the confirmed flow does not require.
- If the log content is missing, respond that the input log is required before the flow can be applied.
- Keep the threshold exactly at more than 10 error lines.

## Output behavior

Return a concise operational result that states:
- the number of ERROR lines found,
- which branch was taken,
- and that any Jira, notification, and summary details not specified by the flowchart remain unspecified.

## Notes

This skill intentionally stays narrow. It reflects the diagram literally and does not add incident-management policy beyond the confirmed steps.