---
name: weekly-expense-table
description: Turn a week of daily expense notes into a readable table with per-item categories and end-of-week category totals. Use this when a user wants weekly spending organized from daily entries and summarized by category.
---

# Weekly Expense Table

## What this skill does
This skill turns a week of daily expense notes into a clean table, keeps each expense labeled with its category, and totals each category at the end of the week.

## When to use it
Use this skill when the user provides daily spending records for a week and wants them organized into a table plus category totals.

## Instructions
1. Read the full week of expense records.
2. Keep every supplied field that matters for the record: date, item or note, amount, category, and any extra note.
3. If the input is already structured, preserve its structure as much as possible while making it easier to read.
4. If the input is unstructured, convert it into a table with clear columns such as:
   - Date
   - Item or note
   - Category
   - Amount
   - Remarks, if present
5. Group the records by day so the output shows each day's spending clearly.
6. At the end of the week, calculate totals by category across all records in the week.
7. If the input spans exactly one week, present both:
   - daily detail rows
   - category totals for the week
8. If required information is missing, do not guess. Ask for the missing date, amount, or category before completing the table.
9. Do not invent categories. Use the categories provided by the user, or ask for clarification if a record has no category.
10. Keep the output concise and easy to scan.

## Output format
Use two sections:

### 1) Daily expenses
Provide a table with one row per expense.

### 2) Weekly totals by category
Provide a summary table with each category and its total amount for the week.

## Quality checks
Before finishing, verify that:
- every expense from the input appears once in the daily table
- every expense has a category label
- category totals add up to the listed expenses
- missing required fields are reported instead of guessed