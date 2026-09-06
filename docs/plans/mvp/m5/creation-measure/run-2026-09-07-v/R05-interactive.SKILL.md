---
name: weekly-expense-table-summary
description: 將一週每日花費整理成表格，當需要把支出記錄成可讀表格並在週末彙總各類別合計時使用。
---

# Weekly expense table summary

## Purpose
Turn one week of daily expense records into a table, label each expense with a category, and total each category at the weekend.

## Instructions
1. Read the input expense records for one week.
2. Make a table that includes each expense entry.
3. Add a category label to every expense entry.
4. Keep the weekday and amount for each entry.
5. At the weekend, calculate the total for each category.
6. Present the category totals clearly after the table.
7. Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
8. Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Output shape
- A table of the week’s expense entries.
- A weekend section that lists each category and its total.

## If input is incomplete
- If a needed field is missing from the input, write 'not given' for that field.
- Do not invent missing categories, dates, or amounts.