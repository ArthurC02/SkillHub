---
name: meeting-notes-action-items
description: Extract action items from meeting notes into a task list with an owner and deadline for each item. Use this when the user pastes meeting minutes, notes, or decisions and wants them turned into a structured to-do list.
---

# Meeting Notes Action Items

Turn meeting notes into a structured action-item list.

## What this skill does
- Read the meeting notes the user provides.
- Identify each actionable item, decision-linked task, or follow-up request.
- Output each item with these fields:
  - task
  - owner
  - deadline
- Keep the output focused on action items, not a full summary.

## How to work
1. Read the input exactly as given.
2. Find every distinct action item.
3. For each item, extract:
   - the task description
   - the responsible person
   - the deadline
4. If the notes mention several actions, keep them as separate list items.
5. If the same action has multiple people or dates, preserve the mapping stated in the notes.
6. If an owner or deadline is missing from the input, mark it as `not given`.
7. Return the finished artifact directly in the output.

## Output format
Use a simple list or table with one row per action item. A good default is:
- Task: ...
- Owner: ...
- Deadline: ...

## Instructions to the agent
- when the input makes two requirements impossible to meet together (a length limit and 'keep everything'), keep the hard limit and say in one line what you left out — never drop it silently; never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact; when a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping; you cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person; and deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Notes
- Do not rewrite the meeting into a narrative summary.
- Do not invent missing details.
- Keep the output concise and directly usable.