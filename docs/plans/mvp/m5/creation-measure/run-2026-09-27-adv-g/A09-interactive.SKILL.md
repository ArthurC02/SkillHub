---
name: overtime-hours-highlighter
description: Calculate each person's monthly overtime hours from attendance records and flag anyone whose overtime exceeds 46 hours. Use this when you are given attendance data and need a direct per-person total plus an over-threshold mark.
---

# Overtime Hours Highlighter

You calculate each person's overtime hours from the attendance record the user gives you, then mark anyone whose overtime is greater than 46 hours.

## Rules to follow

- never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact;
- when a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping;
- you cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## What to do

1. Read the attendance record the user provided.
2. Identify each person and gather every overtime entry that belongs to that person.
3. If one person appears more than once, add their overtime values together.
4. If the record uses a format that does not explicitly say how to total overtime, use the common default for the given data shape and state that default in the result.
5. Compute each person's total overtime hours.
6. Mark any person whose total is strictly greater than 46 hours as over the threshold.
7. Present the result as a finished output the user can use immediately.

## Output format

Use a clear list or table with these fields for each person:

- name
- total overtime hours
- over 46 hours: yes/no

If a value is missing from the input, write `not given`.

If the input already gives a complete monthly overtime total per person, use that total directly.

If the input gives multiple attendance rows per person, sum them before checking the 46-hour threshold.

If the user asks for a shareable or sendable version, format the result cleanly and leave sending or scheduling to the person.
