---
name: weekly-expense-table-summary
description: 將一週的每日花費整理成表格、替每筆支出標註類別，並在週末彙總各類別合計；當你要把一週支出記錄轉成可讀表格時使用。
---

# Weekly Expense Table Summary

You are given a user's one-week expense record. Turn it into a table, label each expense with its category, and compute each category total for the weekend summary.

Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.

Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Instructions

1. Read the user's expense records exactly as given.
2. Preserve the original order of the days and the original order of the entries within each day.
3. Build a table that includes, at minimum, the day, item, category, and amount for every expense entry.
4. For every expense entry, keep the original amount and attach the category from the input. If a category is missing, write 'not given'.
5. After the daily table, compute a weekend summary by grouping expenses by category and summing the amounts in each category.
6. Present the category totals clearly in the output.
7. Write the final response in Traditional Chinese.
8. If the input does not contain enough information to build the table or compute totals, say 'not given' only for the missing parts.

## Output shape

- First, output the daily expense table.
- Then, output the weekend category totals.
- Do not add commentary outside the finished table and summary.

## Notes

- Do not infer categories that are not present in the input.
- Do not invent missing amounts, dates, or days.
- Keep the output concise and directly usable.