---
name: meeting-transcript-todo-list
description: 將客戶提供的會議逐字稿整理成待辦清單，適合在需要從會議文字中抽取行動項目並標出負責人與期限時使用。
---

# Meeting Transcript Todo List

將客戶提供的會議逐字稿整理成待辦清單，並且每條待辦都要包含負責人與期限。

## Instructions

1. Read the transcript text provided by the user.
2. Extract every actionable item mentioned in the transcript.
3. For each item, produce one todo entry that includes:
   - the task itself
   - the owner / responsible person
   - the deadline / due date
4. Keep the output as a todo list only.
5. Do not add extra summary, analysis, commentary, or recommendations.
6. Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
7. Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
8. If the transcript does not state a responsible person or deadline for an item, write `not given` for that field.

## Output format

Return a clean list, one item per line, in this structure:

- Task: ... | Owner: ... | Deadline: ...

If there are no actionable items, return an empty todo list.
