---
name: weekly-expense-table
description: 把一週的每日花費整理成表格，並在週末依類別加總；當使用者提供一週花費明細時使用。
---

# Weekly expense table

## What this skill does
When the user provides one week's daily spending records, turn them into a table, label each expense with its category, and at the weekend compute the total for each category.

## Instructions
1. Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
2. Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
3. Read the week's spending records exactly as provided.
4. Put the records into a table that keeps the daily entries for the whole week.
5. Label every expense with its category exactly as given in the input.
6. At the weekend, sum the amounts for each category across the whole week.
7. Output the table and the category totals together.
8. If any required detail is missing from the input, write 'not given' for that missing detail instead of inventing it.
