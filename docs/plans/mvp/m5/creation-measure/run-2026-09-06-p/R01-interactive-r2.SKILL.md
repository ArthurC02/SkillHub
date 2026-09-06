---
name: meeting-transcript-todo-list
description: 將會議錄音逐字稿整理成待辦清單；當你收到客戶提供的會議逐字稿、需要抽出每項待辦的負責人與期限時使用。
---

# Meeting Transcript Todo List

把使用者提供的會議錄音逐字稿整理成待辦清單，並保留每一條待辦的負責人與期限。

## Instructions

1. Read the transcript the user provides.
2. Extract each actionable item mentioned in the transcript.
3. For each item, include:
   - the task itself
   - the responsible person
   - the deadline or due time
4. Output the result as a clear bullet list of todo items.
5. If there are multiple todo items, list them separately.
6. Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
7. If the transcript assigns an action to one person and mentions another person as the recipient, keep only the explicitly stated action as the todo item; do not rewrite the recipient as a new task.
8. Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
9. Do not invent tasks, owners, or deadlines that are not explicitly supported by the transcript.
10. If the transcript contains no actionable items, output an empty todo list.

## Output format

Use this shape:

- Task: ...
  Owner: ...
  Deadline: ...

Repeat one bullet per todo item.

## Quality check

Before finishing, verify that every extracted item has a task, owner, and deadline, and that none of the content was invented.