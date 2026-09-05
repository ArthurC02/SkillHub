---
name: excel-customer-list-cleaner
description: Clean an Excel customer list by removing duplicate rows and marking rows with missing phone numbers. Use this skill when you need a repeatable workflow for deduplicating customer data and flagging incomplete contact records in Excel.
---

# Excel Customer List Cleaner

## What this skill does
- Reads a customer list from an Excel workbook.
- Removes duplicate rows based on the duplicate-key columns you specify.
- Flags rows where the phone field is empty or missing.
- Produces a cleaned result and a clearly marked missing-phone result.

## When to use it
Use this skill when you have an Excel customer list and need to:
- deduplicate records consistently,
- identify incomplete rows that lack phone numbers,
- keep the result traceable back to the original rows.

## Required inputs
Before processing, identify:
1. The Excel file to process.
2. Which worksheet or table to use, if the workbook contains more than one.
3. Which column(s) define a duplicate record.
4. Which column contains the phone number.

If any of these are missing, stop and ask for the missing detail instead of guessing.

## Processing steps
1. Open the workbook and select the target worksheet or table.
2. Inspect the header row to confirm the available columns.
3. Confirm the duplicate-key columns and the phone column.
4. Normalize the duplicate keys only as needed to compare exact values consistently:
   - trim leading and trailing whitespace,
   - treat blank cells as blank,
   - do not infer matches from unrelated columns.
5. Remove duplicate rows using the confirmed duplicate-key columns.
6. Mark every row where the phone field is blank or missing.
7. Keep a trace column or equivalent row reference so the cleaned result can be matched back to the original row.

## Output requirements
Return two clearly separated results:
- a deduplicated customer table,
- a table or marked view showing rows with missing phone numbers.

The output should also include:
- the duplicate-key columns used,
- the phone column used,
- the number of rows removed as duplicates,
- the number of rows flagged for missing phone numbers.

## Rules and limitations
- Do not assume column names.
- Do not assume a duplicate definition if it is not provided.
- Do not guess the phone column if it is not explicitly identified.
- If the workbook has multiple sheets or tables, process only the one the user identifies.
- If the input is not an Excel file, ask for a valid Excel workbook.
- If the data contains formulas, use the displayed cell values for comparison unless the user explicitly asks otherwise.
- Preserve original row traceability wherever possible.

## Quality check
Before finishing, verify that:
- duplicate rows were removed according to the stated keys,
- rows with missing phone values are visibly marked,
- the result still maps back to the original source rows,
- no column was assumed without confirmation.