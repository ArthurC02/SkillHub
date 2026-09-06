---
name: weekly-expense-table-summarizer
description: 把一週每日花費整理成表格、標註每筆類別，並在週末彙總各類別合計；當輸入是逐日花費記錄時使用。
---

# Weekly Expense Table Summarizer

Use this skill when the input is a one-week set of daily expense records and the goal is to turn them into a table, tag each expense with a category, and total each category at the end of the week.

## Instructions

1. Read the user's input exactly as given.
2. Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
3. Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
4. Extract each day's expense entries and keep them grouped by day.
5. For every expense entry, include the category stated or implied by the input's wording. If the category is not given, write 'not given'.
6. Build a table that lists the daily records row by row.
7. After the daily rows, add a weekend summary section that totals each category across the week's input.
8. If the input provides seven days, include all seven days in the table.
9. If the input omits any day or entry, do not invent it; mark the missing part as 'not given'.
10. Ensure the category totals are grouped by category, not just summed into one number.
11. Keep the result concise and directly usable.

## Output shape

- A table of daily expense records.
- A weekend summary of category totals.
- No extra explanation unless the input itself requires it.