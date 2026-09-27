---
name: monthly-overtime-flagger
description: Calculate each person's overtime hours from this month's attendance records and flag anyone whose overtime exceeds 46 hours. Use this when you need a one-pass summary of overtime totals with clear over-threshold marking.
---

# Monthly Overtime Flagger

Use this skill when the input is this month's attendance record data and you need to calculate each person's overtime hours, then mark anyone whose overtime is above 46 hours.

## Instructions

1. Read the attendance records in the input and identify each person.
2. Calculate each person's overtime hours from the record data provided.
3. Group records by person and total the overtime hours for that person. Treat the grouped records as the calculation step, and output the summed overtime total rather than echoing line-by-line input data.
4. Mark any person whose overtime total is greater than 46 hours.
5. Do not mark people whose overtime total is exactly 46 hours as over the threshold.
6. Output every person's name and overtime total.
7. For every person whose overtime total is greater than 46 hours, add a dedicated field or column that explicitly says they are over 46 hours, using the same flag text for every such person.
8. Make the over-46-hour flag obvious and separate from the numeric total.
9. If the input format does not explicitly state how overtime is represented, use the most common interpretation in the record and finish the calculation rather than stopping.
10. If any required fact is missing from the input, write `not given` for that missing fact instead of inventing it.
11. If the input conflicts with a hard limit and the request says to keep everything, keep the hard limit and say in one line what was left out.
12. Deliver the finished result itself in the response; do not describe rules or ask for more data unless the input is actually missing.
13. You cannot send, post, schedule, monitor, or fetch anything, so if the request asks for that, provide the content ready to use and say plainly that sending or scheduling is left to the person.

## Output format

Return a simple list or table with one row per person.

For each person, include:
- name
- overtime total
- whether the total is over 46 hours

Use a clear label such as `OVER 46` or `flagged` for people above the threshold.

## Edge cases

- If two or more records belong to the same person, sum them before checking the threshold.
- If overtime is given as a decimal, keep the decimal in the total.
- If the input contains both overtime and non-overtime attendance fields, use only the data needed to compute overtime and ignore unrelated fields.
- If the input does not provide a person's name, write `not given` for the name.
- If the input does not provide a usable overtime value, write `not given` for the overtime total.

## Quality check

Before finalizing, verify that every person in the input appears in the output and that anyone above 46 hours is clearly flagged.