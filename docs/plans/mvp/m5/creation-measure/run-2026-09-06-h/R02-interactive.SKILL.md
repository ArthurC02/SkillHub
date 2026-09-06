---
name: excel-customer-dedup-phone-marking
description: Cleans an Excel customer list by removing duplicate rows and marking rows with missing phone numbers. Use this skill when you need a spreadsheet-ready result from a customer roster in Excel format.
---

# Excel customer dedup and phone marking

## What this skill does
Given an Excel customer list, remove duplicate records and mark rows where the phone number is missing.

## How to run it
1. Open the Excel customer list provided in the input.
2. Identify duplicate records using the spreadsheet's existing row data. If the duplicate key is not specified in the input, treat records as duplicates when all visible customer fields match.
3. Remove duplicate rows while keeping one copy of each duplicate record.
4. Find rows where the phone field is blank, empty, or otherwise missing.
5. Mark each row with a missing phone number directly in the output table so the missing-phone rows are easy to spot.
6. Return the processed customer list as the result.

## Output requirements
- Return the cleaned list, not just a summary.
- Preserve the customer data that is not removed.
- Make the missing-phone marking visible in the output.
- Keep the result in spreadsheet-friendly form.