---
name: attendance-overtime-summary
description: Calculate each person's overtime hours from a monthly attendance record and flag anyone whose total exceeds 46 hours. Use this when you need a per-person overtime summary from attendance data and a clear over-threshold marker.
---

# Attendance overtime summary

You turn one monthly attendance record into a per-person overtime summary and mark anyone whose total exceeds 46 hours.

## What to do

1. Read the input as the working record for one month.
2. Use the overtime values provided in the record, or if the record gives enough information to compute overtime directly, compute each person's total overtime from it.
3. Sum each person's overtime hours for the month.
4. Mark every person whose total is strictly greater than 46 hours.
5. Output the result for every person in the input.

## Assumptions to use when the input does not say otherwise

- Treat "over 46 hours" as strictly greater than 46, not equal to 46.
- Use the overtime data already present in the record when it is provided.
- If the record can be computed directly, do that instead of stopping to ask for more detail.
- Summarize by person; do not add scheduling, payroll, or performance analysis.

## Required behavior

- Keep every person from the input in the output.
- Show each person's total overtime hours.
- Clearly label only the people whose total is above 46 hours.
- If the input contains multiple people, process them all in one pass.

## Output shape

Use a clear per-person list or table with at least:

- person name
- total overtime hours
- a marker for totals above 46 hours

If the input format is already tabular, preserve a table-like output.
If the input is plain text, return a plain-text summary that is easy to scan.

## If the input is missing needed data

If the record does not contain enough information to total overtime for a person, say that the data is not given for that person and continue with the rest of the record.

## Finish the work

Deliver the finished overtime summary directly in the output. Do not describe these rules instead of producing the result.