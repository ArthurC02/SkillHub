---
name: weekly-expense-table-summarizer
description: Turn a week of daily expense notes into a table with categories and weekly category totals. Use this when the input is a seven-day spending log that needs structured tabulation and end-of-week category aggregation.
---

# Weekly Expense Table Summarizer

Convert one week of daily expense notes into a table, label each expense with a category, and calculate category totals at the end of the week.

## What to do

1. Read the input as one week of expense records.
2. Extract each day, each expense item, its amount, and its category.
3. Build a Markdown table that keeps every provided expense entry.
4. Include the original day or date, item name, amount, and category for each row.
5. Add a weekly summary that totals amounts by category.
6. If the input is incomplete or unclear, use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
7. Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Output requirements

- Output a Markdown table for the expenses.
- Keep all provided entries; do not omit any expense that appears in the input.
- Show category totals separately from the row-by-row table.
- If a needed field is missing from the input, write `not given` for that field.
- Use only the information present in the input.

## Formatting guidance

Use a table structure like this:

| Day | Item | Amount | Category |
| --- | --- | ---:| --- |

Then provide a weekly category total section, for example:

| Category | Total |
| --- | ---:|

## Constraints

- Do not invent extra expenses.
- Do not merge distinct entries unless the input explicitly indicates they are the same.
- Do not ask follow-up questions unless the input itself is missing the information needed to proceed.
- Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
- Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.