---
name: excel-customer-cleanup
description: Clean an Excel customer list by removing duplicate rows and flagging rows missing a phone number. Use this skill when you are given a customer spreadsheet that needs deduplication and a clear missing-phone marker in the output.
---

# Excel customer cleanup

## What this skill does
Process a customer list from Excel, remove duplicate customer rows, and mark rows that are missing a phone number.

## When to use it
Use this skill when the input is an Excel workbook or worksheet containing customer data and the goal is to produce a cleaned spreadsheet with duplicates removed and missing phone numbers clearly flagged.

## Operating rules
1. Load the input workbook and identify the data table containing customer rows.
2. Treat a row as a duplicate when the combination of `姓名`, `公司`, and `電話` matches an earlier row exactly after trimming surrounding whitespace.
3. Keep the first occurrence of each duplicate group and remove later duplicates.
4. Add a new column named `電話狀態`.
5. Set `電話狀態` to `缺少電話` when the `電話` cell is empty, blank, or contains only whitespace.
6. Set `電話狀態` to `正常` for rows that have a phone number.
7. Preserve the original column order as much as possible, inserting `電話狀態` after `電話` when that column exists; otherwise append it to the end.
8. Preserve the workbook structure and sheet data in a spreadsheet-friendly form rather than converting the result into plain text.

## Output requirements
- Return the cleaned Excel file as the primary output.
- Include the cleaned rows, not just a summary.
- Make the missing-phone marking visible in the spreadsheet itself.

## Handling details
- If the sheet has headers, use the header row to find the relevant columns.
- If there are multiple sheets, process the sheet that contains the customer table and leave unrelated sheets unchanged unless the surrounding task indicates otherwise.
- If the input already contains a `電話狀態` column, overwrite its values consistently with the rules above.
- If the workbook contains formulas, keep the cell values intact unless the spreadsheet engine requires recalculation.

## Verification checklist
- Duplicate rows based on `姓名 + 公司 + 電話` are removed.
- Rows with missing phone numbers are marked with `電話狀態 = 缺少電話`.
- Rows with phone numbers are marked with `電話狀態 = 正常`.
- The output remains an Excel workbook with readable table structure.