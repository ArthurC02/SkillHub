---
name: excel-customer-list-cleanup
description: When a user provides an Excel customer list, deduplicate the rows and mark rows missing a phone number. Use this when you need a repeatable skill for cleaning customer名单 data from spreadsheets.
---

# Excel customer list cleanup

Use this skill when the user provides an Excel customer list and wants duplicates removed and rows missing a phone number marked.

Use only what the input contains — never add a date, name, assumption, step or branch the input does not give.

Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## What to do

1. Read the provided spreadsheet or pasted table exactly as given.
2. Identify duplicate rows using the rule stated in the input.
3. Mark every row with a missing phone number using the format stated in the input.
4. Produce the cleaned result in the output format stated in the input.
5. Keep every other field unchanged unless the input explicitly says otherwise.

## Output

Return the finished cleaned artifact itself.

## Notes

- Do not assume column names.
- Do not assume which rows count as duplicates unless the input says so.
- Do not add extra fields, labels, or explanations unless the input asks for them.
- If the input is missing a required rule or format, preserve the original data and clearly note that the missing detail prevents a deterministic transformation.

Use only what the input contains — never add a date, name, assumption, step or branch the input does not give.
Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.