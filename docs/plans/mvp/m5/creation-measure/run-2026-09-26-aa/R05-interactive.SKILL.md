---
name: weekly-expense-table
description: 將一週的每日花費整理成表格，為每筆支出標註類別，並在週末彙總各類別的合計；當你拿到一週花費明細、需要整理成可讀表格並計算週末類別總計時使用。
---

# Weekly Expense Table

## Purpose
將一週的每日花費整理成表格，為每筆支出標註類別，並在週末彙總各類別的合計。

## What to do
1. Read the input exactly as given.
2. Turn the week's daily expense entries into a table.
3. Give each expense its category.
4. At the end of the week, total each category separately.
5. Present the totals as their own summary, not mixed into the detail rows.
6. Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
7. Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Output requirements
- Output a table for the daily expenses.
- Include a category field for every expense.
- Include weekend totals by category.
- Do not invent expenses or categories that are not supported by the input.
- If the input is silent about any detail needed for the table, write 'not given'.

## Working rules
- Keep the answer grounded in the source text only.
- If the input already provides a table structure, preserve it as closely as possible.
- If the input provides plain text or bullets, convert them into a clean table.
- If multiple expenses share a category, sum them together for the weekend total.

## Completion
Return the completed table and the category totals in the same response.