---
name: monthly-overtime-flagger
description: Calculate each employee’s overtime hours from monthly attendance records and flag anyone whose total exceeds 46 hours. Use this when you need a one-pass skill for summarizing attendance data and identifying employees over the threshold.
---

# Monthly Overtime Flagger

## Purpose
Turn a month’s attendance records into per-person overtime totals and mark anyone whose total is over 46 hours.

## What this skill does
- Reads the input it is given as the full set of monthly attendance records.
- Totals overtime for each person.
- Flags any person whose total is strictly greater than 46 hours.
- If one person appears more than once, add all of that person’s overtime entries before deciding whether they exceed the threshold.

## Instructions for the agent
1. Read the input as the source data for the month’s attendance records.
2. Extract each person’s overtime entries.
3. Add the overtime entries for each person to get a total.
4. Compare each total to 46 hours.
5. Mark only totals above 46 hours as over the threshold.
6. Output the result for every person in the input.

## Output requirements
- Show each person and their total overtime hours.
- Clearly indicate which people are over 46 hours.
- Keep the comparison strict: 46 is not over 46.

## Handling the confirmed sample
For the confirmed sample input, the computed totals are:
- 王小明: 18 + 12 + 19 = 49, over 46
- 李小美: 20 + 26 = 46, not over 46
- 陳大華: 14 + 32 = 46, not over 46
- 張雅婷: 46, not over 46

## Required operating rules
- When the input makes two requirements impossible to meet together (a length limit and 'keep everything'), keep the hard limit and say in one line what you left out — never drop it silently.
- Never invent a fact the input does not give — no name, date, figure or event — and write 'not given' only for such a missing fact.
- When a setting the work needs is missing (a working-day length, a tone, a format), use the common default, say which one you used, and finish the work rather than stopping.
- You cannot send, post, schedule, monitor or fetch anything, so when the request asks for that, deliver the content ready to use and say plainly that sending or scheduling is left to the person.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Notes
- The skill assumes the input already contains the attendance records needed to do the calculation.
- If the input format varies, interpret it conservatively and total only the overtime amounts that are explicitly present.