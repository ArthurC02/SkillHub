---
name: meeting-transcript-todo-list
description: Turn a client-provided meeting transcript into a to-do list with task, owner, and deadline. Use this when the input is a transcript and the goal is a concise action list in Traditional Chinese.
---

# Purpose
Convert a meeting transcript into a concise to-do list in Traditional Chinese.

# When to use
Use this skill when the user provides a meeting transcript and wants the action items organized as a to-do list with an explicit owner and deadline for each item.

# Instructions
1. Read the transcript exactly as provided.
2. Extract only action items that are explicitly supported by the transcript.
3. For each item, include these three fields:
   - Task
   - Owner
   - Deadline
4. Keep the output in Traditional Chinese.
5. Do not invent tasks, owners, or deadlines.
6. If an item does not explicitly include an owner or a deadline, omit that item rather than filling the missing field.
7. Preserve the meeting meaning from the transcript, but do not add extra action items, assumptions, or interpretations.
8. Present the result as a clean to-do list, using either bullets, numbering, or a table, as long as each retained item clearly shows all three fields.

# Output requirements
- Output only the completed to-do list.
- Do not explain the rules.
- Do not mention omitted items.
- Keep wording natural and concise in Traditional Chinese.

# Example
Input transcript:
- Proposal to the client by Friday, owner Xiao Lin.
- Confirm the quotation sheet by next Monday, owner A-Mei.
- Compile the meeting minutes by Friday, owner Xiao Lin.

Expected result:
1. 小林｜提案寄給客戶｜週五前
2. 阿美｜確認報價單｜下週一前
3. 小林｜彙整會議紀錄｜週五前