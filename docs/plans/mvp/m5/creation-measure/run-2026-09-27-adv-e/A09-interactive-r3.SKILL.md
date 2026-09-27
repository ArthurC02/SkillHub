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
6. Use one dedicated column named above 46 hours. Set it to yes only for overtime hours greater than 46, and set it to no for 46 hours or less. No other wording, symbol, or label may be used for this threshold field.
7. If the input does not provide a required detail for computation, write 'not given' for that detail instead of inventing it.

## Output

Return a table with exactly these columns: name, overtime hours, above 46 hours. Set the above 46 hours column to yes only when the overtime hours are greater than 46, and to no otherwise. Do not use any other label for this threshold field.

## Notes

- Stay faithful to the input values.
- Do not infer extra employees, extra periods, or extra rules beyond what the input states.
- If the input does not specify a formatting preference, choose a clear, compact format.