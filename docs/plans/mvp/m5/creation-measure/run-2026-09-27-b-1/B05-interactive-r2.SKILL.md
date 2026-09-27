---
name: meeting-notes-todo-list
description: 將使用者貼上的會議記錄整理成待辦事項清單，並在每項中標出負責人與期限；適合在需要把會議紀錄快速轉成可執行行動項時使用。
---

# Meeting notes to-do list

將使用者提供的會議記錄整理成可執行的待辦事項清單。輸出時以原文為準，優先保留原文中可辨識的行動項、負責人與期限；若原文沒有寫出負責人或期限，就在對應欄位寫 `未指定`；不要改寫成其他字樣。

## How to work

1. Read the meeting record the user provided.
2. Identify each distinct action item, decision that requires follow-up, and assigned task.
3. For each item, extract:
   - **事項**: the task itself.
   - **負責人**: the person or group named in the meeting record.
   - **期限**: the date, time, or relative deadline named in the meeting record.
   - **備註**: any short note needed to preserve context from the record, such as dependencies or wording that matters.
4. Keep only items supported by the meeting record. Do not add tasks that are not present.
5. If a task is mentioned but the owner or deadline is missing, keep the task and use `未指定` in the missing field.
6. Present the result as a clear list or table that is easy to scan.

## Output shape

Use a simple table when possible:

| 事項 | 負責人 | 期限 | 備註 |
| --- | --- | --- | --- |
| ... | ... | ... | ... |

If a table would be awkward for the input, use a bulleted list with the same four fields for every item.

## Required operating rules

when the input makes two requirements impossible to meet together (a length limit and 'keep everything'), keep the hard limit and say in one line what you left out — never drop it silently;

never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact;

when a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping;

you cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person;

deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Decision guidance

- Treat a bullet, sentence, or clause as a separate task only when it clearly describes a distinct follow-up.
- If the record names a department rather than a person, keep that department as the 負責人.
- If the record gives a relative deadline such as “下週三前”, preserve it as written instead of converting it.
- If several tasks share the same owner or deadline, repeat that information for each row so the output stays self-contained.
- If the record includes both decision items and action items, include only the items that require follow-up work.

## Final check

Before responding, verify that every extracted item has the four fields and that nothing in the output depends on outside knowledge.