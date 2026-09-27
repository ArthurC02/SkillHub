---
name: monthly-overtime-summary
description: Calculate each person's overtime hours from a month's attendance records and flag anyone over 46 hours. Use it when the user gives attendance data and needs a per-person overtime total with over-threshold marking.
---

# Monthly Overtime Summary

Use this skill when the user gives a month of attendance records and asks for each person's overtime total, with anyone over 46 hours clearly marked.

## What to do
1. Read the attendance records the user provides.
2. Group the records by person.
3. Add up each person's overtime hours from the provided records.
4. Mark any person whose total overtime is greater than 46 hours.
5. Return the full per-person summary, not only the over-46 list.

## Output requirements
- Show each person's name and total overtime hours.
- Show whether the total is over 46 hours.
- If the data format is unclear, use the common default of a simple table-based summary and finish the work.
- If the input leaves out a fact you need, write `not given` only for that missing fact.

## Rules
- When the input makes two requirements impossible to meet together, keep the hard limit and say in one line what you left out — never drop it silently.
- Never invent a fact the input does not give — no name, date, figure or event — and write `not given` only for such a missing fact.
- When a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping.
- You cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Process
1. Identify each person's records in the input.
2. Sum overtime hours for each person.
3. Compare each total with 46 hours.
4. Present the results in a clear table or list.
5. Mark any total above 46 hours as over the threshold.

## Completion check
- Every person in the provided attendance records appears in the result.
- Each person has a total overtime figure.
- Any total above 46 hours is explicitly marked.
- Totals at or below 46 hours are not marked as over the threshold.
- The output uses only the information in the input.