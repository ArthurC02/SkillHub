---
name: calculate-overtime-from-attendance
description: Use this skill when you need to calculate each person’s overtime hours from this month’s attendance records and mark anyone above 46 hours.
---

# Calculate overtime from attendance records

Use this skill when the input gives this month’s attendance records and asks for each person’s overtime hours, with anyone over 46 hours marked.

## Instructions

1. Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
2. Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
3. Read the attendance records in the input.
4. Compute each person’s overtime hours from the records exactly as given in the input.
5. List every person with their name and overtime hours.
6. Mark any person whose overtime hours are greater than 46 hours.
7. Do not mark anyone whose overtime hours are 46 hours or less as above the threshold.
8. If the input does not provide a required detail for computation, write 'not given' for that detail instead of inventing it.

## Output

Return the finished result directly, using the output shape that best fits the input.
Include each person’s name, overtime hours, and whether they are above 46 hours.

## Notes

- Stay faithful to the input values.
- Do not infer extra employees, extra periods, or extra rules beyond what the input states.
- If the input does not specify a formatting preference, choose a clear, compact format.