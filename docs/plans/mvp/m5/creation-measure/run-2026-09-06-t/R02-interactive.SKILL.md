---
name: excel-customer-list-cleaner
description: Clean Excel customer lists by removing duplicates using the columns the user specifies and marking rows with missing phone numbers. Use this when you receive a table or spreadsheet-like customer list that needs deduplication and a phone-missing flag in one pass.
---

# Excel customer list cleaner

Use this Skill when the input is a customer list in Excel-like tabular form and the task is to remove duplicates and mark rows with missing phone numbers.

## What to do
1. Read the customer list exactly as provided.
2. Use the columns the input names to decide what counts as a duplicate. If the input does not say which columns define duplicates, write `not given` for that rule instead of inventing one.
3. Mark every row whose phone field is blank, empty, or missing.
4. Keep the non-duplicate rows and preserve the rows that have complete phone data.
5. Return the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Required output
Provide results in a form that can be applied directly in Excel, such as a cleaned table plus an added note or flag for missing phone numbers.

## Constraints
- Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
- If the input does not provide enough information to identify the duplicate key or the phone column, state `not given` for the missing part and continue only with the information that is present.
- Do not ask follow-up questions when the input already contains the needed data.

## Output structure
- A cleaned customer list with duplicates removed.
- A clear marker for rows with missing phone numbers.
- A brief note naming the duplicate rule and the phone column, or `not given` where either is absent.