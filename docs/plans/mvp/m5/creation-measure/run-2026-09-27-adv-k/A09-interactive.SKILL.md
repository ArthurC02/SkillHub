---
name: monthly-overtime-summary
description: Calculate each employee’s monthly overtime from attendance records and flag anyone whose overtime exceeds 46 hours. Use this when the user provides attendance data and needs a per-person overtime summary plus an over-threshold list.
---

# Monthly overtime summary

Compute each person’s monthly overtime from the attendance records in the input, then flag anyone whose total overtime is greater than 46 hours.

## Instructions

1. Read the input as the only source of truth.
2. Identify the overtime field or the information needed to derive overtime from the provided attendance records.
3. Sum overtime per person for the month.
4. Mark any person whose total overtime is greater than 46 hours.
5. Include every person who appears in the input.
6. If the input does not provide enough information to compute overtime, state what is missing and stop.
7. Output a clear per-person summary and a separate over-46-hours list, if any.
8. Keep the calculation transparent enough that the result can be checked against the input.

## Working rules

- When the input makes two requirements impossible to meet together, such as a length limit and “keep everything,” keep the hard limit and say in one line what you left out — never drop it silently.
- Never invent a fact the input does not give — no name, date, figure or event — and write “not given” only for such a missing fact.
- When a setting the work needs is missing, such as a working-day length, a tone, or a format, use the common default, say which one you used, and finish the work rather than stopping.
- You cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Output shape

Provide:
- a per-person overtime table or list
- a clearly labeled over-46-hours section
- a brief note if any required input was missing or any assumption was used

If the input already contains a total overtime field, use it directly. If it contains daily overtime values, sum them. If it only contains raw attendance data and the overtime rule is not stated, do not guess the rule; report that the rule is not given.