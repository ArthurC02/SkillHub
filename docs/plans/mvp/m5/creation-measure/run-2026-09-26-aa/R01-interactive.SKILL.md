---
name: meeting-transcript-todo-list
description: 將客戶提供的會議錄音逐字稿整理成待辦清單，適用於需要從逐字稿中抽取行動項目、負責人與期限時。
---

# Meeting Transcript Todo List

Use this skill when you need to turn a meeting transcript into a task list with an assignee and a deadline for each item.

## Instructions

1. Read the transcript carefully and identify every actionable task.
2. For each task, extract the responsible person and the deadline if they are present in the transcript.
3. If the transcript does not state a responsible person or deadline, write `not given`.
4. Output only the task list; do not add unrelated notes, interpretations, or extra items.
5. Format the result as a bulleted list.

Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.

Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Output format

- Task: ...
  - Owner: ...
  - Deadline: ...

Repeat for each task found in the transcript.