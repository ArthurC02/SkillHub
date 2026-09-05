---
name: weekly-expense-table-summary
description: Turns a week of daily expense records into a table with a category for each entry and a weekend summary of totals by category. Use when you need to organize weekly spending into a readable ledger and category totals.
---

# Weekly Expense Table Summary

## What this skill does
Turn a week of daily expense records into a table that lists each expense entry, labels every entry with a category, and adds a weekend summary of totals by category.

Use this skill when the user provides daily spending notes for a week and wants them organized into a table with category totals at the end of the week.

## Inputs
Expect records that may include:
- date
- amount
- category
- optional note

If any of these are missing, keep the missing field blank rather than guessing. Do not invent expenses, categories, or dates.

## Output format
Produce a Markdown table with one row per expense entry. Include at least these columns:
- Date
- Amount
- Category
- Note

After the daily rows, add a weekend summary section with one total per category for the week. If the input includes fewer than seven days, summarize only the provided records.

## Steps
1. Read the week’s expense records in chronological order.
2. Keep each expense as its own row.
3. Ensure every row shows a category value if provided; if not provided, leave it blank.
4. Group all entries by category and add the amounts for the weekend summary.
5. Present the daily table first, then the category totals.

## Rules
- Do not add expenses that were not provided.
- Do not infer missing dates, amounts, or categories.
- Do not merge separate expense entries into one row.
- Preserve the user’s original wording for notes when possible.
- If the user provides currency or locale conventions, keep them consistent throughout the table.

## Validation checklist
- Every provided expense appears as one table row.
- Every row includes a category column.
- Category totals are shown at the end of the week.
- No invented expenses appear in the output.
- Missing fields remain blank instead of being guessed.