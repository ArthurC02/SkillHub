---
name: meeting-transcript-todo-list
description: Turn a client-provided meeting transcript into a to-do list with task, owner, and deadline. Use this when the input is a transcript and the goal is a concise action list in Traditional Chinese.
---

# Purpose
Convert a client-provided meeting transcript into a to-do list. Use this when the input is a meeting transcript and the requested output is a task list with an owner and deadline for each item.

# Instructions
1. Read the transcript exactly as provided.
2. Extract each actionable item stated in the transcript.
3. For each item, output three fields:
   - Task
   - Owner
   - Deadline
4. Keep the output in Traditional Chinese.
5. Do not invent tasks, owners, or deadlines.
6. If the transcript does not state an owner or deadline for an item, write `not given` for that field.
7. Preserve the meeting information from the transcript and do not add items that are not supported by the input.
8. Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
9. Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.

# Output format
Produce a clean to-do list. One item per line or in a table is acceptable, as long as every item clearly shows the three required fields.

# Example behavior
Given a transcript that says:
- a proposal must be sent to the client by Friday, owner Xiao Lin
- a quotation sheet must be confirmed by next Monday, owner A-Mei
- the engineering team must add test results, with no owner or deadline stated

The output should list those items only, with missing fields written as `not given`.