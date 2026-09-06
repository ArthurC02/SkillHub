---
name: weekly-expense-table
description: 將一週的每日花費整理成表格，並在需要把每筆支出標註類別、週末加總各類別時使用。
---

# Purpose
Use this skill when a user provides one week of daily expense records and wants them organized into a table with category labels for each expense, plus weekend category totals.

# Instructions
1. Read the user's one-week expense input.
2. Turn the expenses into a table.
3. Label each expense with its category.
4. At the weekend, total each category separately.
5. Preserve the information given by the user.
6. Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
7. Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

# Output
Return the table and the weekend category totals.