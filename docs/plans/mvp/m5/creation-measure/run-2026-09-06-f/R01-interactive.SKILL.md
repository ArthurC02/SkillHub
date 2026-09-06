---
name: meeting-transcript-todo-list
description: 將客戶提供的會議逐字稿整理成待辦清單，當你收到會議逐字稿文字時使用；每條待辦都要整理出事項、負責人與期限，且不自行補寫原文未明說的資訊。
---

# Meeting Transcript Todo List

## Purpose
Turn a meeting transcript into a structured to-do list.
Use this skill when the user provides meeting transcript text and wants action items extracted with assignee and deadline.

## What to produce
Create a to-do list in a clear table or bullet list.
For each action item, include:
- task
- assignee
- deadline

## Working rules
1. Read the transcript once from start to finish and identify explicit action items.
2. Extract only what the transcript states or strongly labels.
3. For each item, preserve the original meaning in concise wording.
4. Include an assignee and deadline for every item:
   - If the transcript explicitly names a person or role, use that assignee.
   - If the transcript does not clearly identify who owns the item, write `未指明`.
   - If the transcript explicitly gives a deadline, use it as written.
   - If no deadline is stated, write `未指明`.
5. Do not invent names, dates, or responsibilities that are not in the transcript.
6. If a transcript line contains multiple tasks, split them into separate to-do items when they can be separated cleanly.
7. If the transcript contains discussion, reminders, or chatter that are not actionable, omit them.

## Output format
Use a table with these columns:
- 待辦事項
- 負責人
- 期限

Keep one row per to-do item.
If the transcript has no actionable items, output a short note stating that no clear to-do items were found.

## Style
- Write in Traditional Chinese.
- Keep wording concise and faithful to the source.
- Do not add explanations unless they are needed to preserve missing assignee or deadline as `未指明`.