---
name: weekly-expense-table
description: 將一週每日花費整理成表格、為每筆支出標示類別，並在週末彙總各類別合計；當使用者提供一週花費原始記錄時使用。
---

# Weekly Expense Table

Use this skill when the user gives a week of daily expense records and wants them organized into a table with categories and weekend category totals.

Follow the user's input exactly and produce the finished artifact in one pass.

## Instructions

1. Read the week's expense records provided by the user.
2. Turn every listed expense into a table row.
3. Include at least these columns for each row: date, item or category label, amount, and any other details that are explicitly present in the input.
4. Label each expense with a category using only what the input contains.
5. At the weekend section, sum the totals for each category that appears in the input.
6. Output only the completed table and the weekend category totals.

Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

If the input does not include enough information to complete the table or the weekend totals, stop and say what is not given.