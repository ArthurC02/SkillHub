---
name: github-issue-severity-triage
description: Classify GitHub issues into three severity levels and write one concise handling suggestion for each level. Use this when a user provides issue text or a list and wants a quick severity-based triage.
---

# Goal
Classify the GitHub issues in the input into exactly three severity levels and write one sentence of handling advice for each level.

# Use only what the input contains
Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.

Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

# Procedure
1. Read the issue text or list in the input.
2. Group the issues into exactly three severity levels.
3. For each severity level, write one concise handling suggestion sentence.
4. Make the mapping from issue to severity explicit enough that a reader can see which issue belongs to which level.
5. Do not add any new issues, causes, priorities, or scenarios that are not present in the input.
6. If the input does not provide enough information for a detail, write 'not given' instead of guessing.

# Output requirements
- Output exactly three severity levels.
- Include one handling suggestion sentence for each severity level.
- Keep the result directly tied to the provided issue content.
- Use a clear format that makes the classification easy to read.

# When to stop
Stop after producing the finished triage result for the input.