---
name: server-log-error-triage
description: Process server logs to count ERROR lines, branch on whether the count exceeds 10, and generate the appropriate Jira, notification, and daily-summary actions. Use this when you need a repeatable workflow for log-based incident triage.
---

# server-log-error-triage

## Purpose
Use this Skill when you have a server log file and need to triage ERROR entries, decide whether the error count exceeds 10, and then follow the correct incident-handling path.

## Inputs
- A server log file or pasted log content.
- The definition of an `ERROR` line in the log format being analyzed.
- Access to the systems needed to create a Jira issue and notify the on-call engineer, if the error threshold is exceeded.

## Procedure
1. Read the server log.
2. Extract all lines that are classified as `ERROR`.
3. Count the extracted `ERROR` lines.
4. Compare the count against the threshold of 10.
5. If the count is greater than 10:
   - Create a Jira issue.
   - Notify the on-call engineer.
   - Write the daily summary.
6. If the count is 10 or fewer:
   - Write the daily summary.

## Output
Produce a concise daily summary that states:
- how many `ERROR` lines were found,
- whether the count exceeded 10,
- and which actions were taken.

If the threshold is exceeded, also include the Jira issue reference and a note that the on-call engineer was notified.

## Constraints
- Do not add extra branches beyond the confirmed threshold check.
- Do not invent additional alerting, escalation, or remediation steps.
- Follow the exact action order: count errors first, then branch on the threshold, then complete the required follow-up actions.

## Tool use
This Skill does not require any specific built-in tools in its instructions. If the execution environment provides Jira or notification capabilities, use those only for the confirmed actions: issue creation and on-call notification when the threshold is exceeded.

## Validation checklist
Before finishing, confirm that:
- the `ERROR` lines were counted from the provided log,
- the threshold comparison used 10 as the cutoff,
- the correct branch was followed,
- and the daily summary was produced.