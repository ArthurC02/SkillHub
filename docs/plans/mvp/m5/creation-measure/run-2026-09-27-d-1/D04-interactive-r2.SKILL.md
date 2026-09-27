---
name: clinic-front-desk-roster-planner
description: Generates a clinic front-desk duty roster for next Monday through Saturday while avoiding approved leave dates for the named staff. Use it when you need a schedule that respects leave constraints and must clearly flag days that cannot be staffed from the provided information.
---

# Purpose
Create a clinic front-desk duty roster for next Monday through Saturday, using the leave dates and roster information provided in the user’s message.

# What to do
1. Read the user’s input as the source of truth.
2. Identify the leave dates for the named staff members.
3. Build the roster for Monday through Saturday only.
4. Do not schedule anyone on a date they have already taken as leave.
5. If the input does not provide enough information to make a complete roster, still produce the best possible roster and clearly mark any day that cannot be assigned.
6. If a day has only one eligible person, assign that person.
7. If a day has no eligible person, mark the day as “待人工確認”.
8. If the user provides enough roster rules to determine assignments, follow them; if not, use the common default of assigning only from the eligible people and state that this default was used.

# Output format
Return a table or list with one row per day from Monday to Saturday.

For each day, include:
- the weekday
- the assigned person, if any
- a note if the day is unassignable or needs manual confirmation

Include Sunday nowhere in the roster.

# Constraints
- Never invent leave dates, staff names, dates, or roster rules that the input does not give.
- If a required fact is missing, mark it as “未提供”.
- If the request and the provided information make it impossible to produce a complete roster, keep the hard limit or explicit constraint and say in one line what was left out.
- When the output lists amounts or quantities that belong together, give their total; work every figure out step by step and add the result up once more before giving it.
- Write the output, labels included, in the language of the input.
- You cannot send, post, schedule, monitor, or fetch anything; produce the roster content ready to use and say plainly that sending or scheduling is left to the person.
- Deliver the finished roster itself in the output, never a description of the rules, a plan, or a request for access.

# Procedure for this task
- Use the exact staff names and leave dates given by the user.
- Exclude any date that overlaps with approved leave.
- If the input mentions a default rule such as “if only one person is available, assign that person,” apply it directly.
- If the input mentions “待人工確認” for no-coverage days, use that label exactly.
- Finish with the roster, not with questions.