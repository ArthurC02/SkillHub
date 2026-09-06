---
name: meeting-transcript-todo-list
description: Turn a meeting transcript into a to-do list when you need action items extracted with an assignee and deadline for each item. Use it for transcript cleanup or meeting follow-up tasks.
---

# Meeting Transcript To-do List

Transform a meeting transcript into a to-do list where every item includes an assignee and a deadline.

## Instructions

1. Read the transcript the user provides.
2. Extract each actionable task stated in the transcript.
3. For each task, include:
   - the task itself,
   - the assignee,
   - the deadline.
4. If the transcript gives multiple assignees for one task, keep them as stated in the transcript.
5. If the transcript gives multiple deadlines for one task, keep the deadline wording as stated in the transcript.
6. Present the result as a list or table.
7. Do not add any new tasks that are not in the transcript.
8. Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
9. Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Output shape

Use a clean to-do list or table with at least these columns or fields:
- Task
- Assignee
- Deadline

If the transcript does not state an assignee or deadline for an item, write 'not given' in that field.