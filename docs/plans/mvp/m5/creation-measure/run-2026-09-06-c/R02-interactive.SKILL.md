---
name: excel-customer-list-cleanup
description: Use this skill when you need to clean a customer list in Excel by removing duplicate customers and marking rows that are missing phone numbers. It is intended for small-to-medium lists such as about 200 rows that need one-pass data cleanup.
---

# Excel Customer List Cleanup

## Purpose
Clean an Excel customer list by removing duplicate customer entries and marking rows that are missing phone numbers.

## Use this skill when
- You have an Excel file with customer records.
- The list may contain duplicate customers.
- Some rows may be missing a phone number and you need those rows marked.
- You want a one-pass cleanup workflow instead of checking rows manually.

## Workflow
1. Open the Excel file and identify the sheet that contains the customer list.
2. Inspect the header row and confirm which columns represent customer identity and phone number.
3. If the list has no obvious unique ID, use the most reliable combination of customer fields available to identify duplicates, such as name plus phone number or name plus email.
4. Remove duplicate rows according to the chosen identity rule.
5. Mark rows that are missing a phone number.
   - If the sheet already has a status or notes column, write a clear marker there.
   - If no suitable column exists, add a new column named `phone-missing` and mark missing-phone rows with a simple value such as `yes`.
6. Keep the original customer data columns intact.
7. Do not invent missing customer data.
8. Save the cleaned result as a new file so the original data remains unchanged.

## Output expectations
- The cleaned Excel output contains the deduplicated customer list.
- Rows missing a phone number are clearly marked.
- Existing customer fields remain readable and unchanged except for the cleanup edits.
- The process is suitable for lists around 200 rows and does not require manual row-by-row editing.

## Notes
- If duplicate rules are ambiguous, choose the safest rule and report the rule used.
- If phone numbers are stored in multiple formats, treat empty cells as missing and do not guess values.
- If the workbook has multiple sheets, only process the sheet containing the customer list unless the user explicitly asks for more.