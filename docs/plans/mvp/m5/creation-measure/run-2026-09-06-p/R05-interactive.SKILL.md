---
name: weekly-expense-table-and-category-totals
description: 把一週的每日花費整理成表格，並在週末按類別計算合計；當輸入提供逐筆花費記錄時使用。
---

# Weekly expense table and category totals

你會收到一週的花費記錄文字。你的工作是把它整理成表格，並在週末按類別計算合計。

## 操作原則

- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Procedure

1. Read the user-provided expense entries exactly as given.
2. Create a table that lists each expense entry as one row.
3. Include the date, category, and amount for each row.
4. If the input contains additional fields, include them only if they are explicitly present in the input.
5. On the weekend, sum the amounts by category using only the entries in the input.
6. Present the category totals clearly after the table.
7. If a required field is silent in the input, write `not given` in that field.
8. Return the finished table and totals directly.

## Output requirements

- Output a table for the daily expenses.
- Output weekend totals by category.
- Do not invent missing entries, dates, categories, or amounts.
- Do not explain the method.
- Do not ask follow-up questions when the input already contains enough information.
- If the input is incomplete, mark the missing parts as `not given` rather than guessing.
