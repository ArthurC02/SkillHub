---
name: server-log-error-triage
description: Use this skill when you need to process a server log, extract ERROR lines, and decide whether to escalate based on an error-count threshold. It helps when the workflow must either file a Jira issue and notify the on-call engineer or only produce a daily summary.
---

# Purpose
Process a server log by extracting `ERROR` lines, counting them, and deciding whether escalation is needed based on a threshold of more than 10 errors.

# When to use
Use this skill when a workflow needs to triage server logs into one of two paths:
- more than 10 `ERROR` lines: create a Jira issue, notify the on-call engineer, and write the daily summary
- 10 or fewer `ERROR` lines: write the daily summary only

# Procedure
1. Read the provided server log file.
2. Extract every line that contains `ERROR`.
3. Count the extracted `ERROR` lines.
4. Compare the count against the threshold of 10.
5. If the count is greater than 10:
   - create a Jira issue
   - notify the on-call engineer
   - write the daily summary
6. If the count is 10 or fewer:
   - write the daily summary only
7. Record the decision path clearly so the output shows both the condition and the resulting branch.

# Output requirements
The skill should produce a result that clearly includes:
- the list or summary of extracted `ERROR` lines
- the error count
- the condition check used for branching
- the actions taken for the selected branch
- the daily summary output

# Assumptions used to fill missing details
- Jira details are represented generically as "create a Jira issue" because the workflow does not specify project keys, fields, or issue templates.
- Notification details are represented generically as "notify the on-call engineer" because the workflow does not specify channel, message format, or recipient lookup rules.
- The daily summary is treated as a plain-text summary artifact because the workflow does not specify format or storage location.

# Limitations and uncertainties
- The log format is not specified; if the log is unstructured, extract lines by literal `ERROR` matching.
- The exact Jira fields are unknown and should not be invented.
- The notification channel and content are unknown and should not be invented.
- The summary destination is unknown and should be left to the host environment or calling workflow.

# Validation checklist
Before using this skill, confirm that:
- a server log file is available as input
- the workflow accepts a threshold of more than 10 errors
- generic Jira creation and notification actions are acceptable when detailed integrations are unavailable
- the daily summary can be produced without a fixed storage target