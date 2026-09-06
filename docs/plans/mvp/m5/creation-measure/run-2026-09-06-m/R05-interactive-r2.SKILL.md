---
name: weekly-expense-table
description: Organize one week of daily expense records into a table with a category for each entry, and use it when you need weekend category totals from user-provided spending logs.
---

# Weekly expense table

Use this skill when the user gives one week of daily expense records and wants them organized into a table with each expense labeled by category, plus weekend totals by category.

## Instructions

1. Read only the expense data the user provides.
2. Turn the records into a table that shows each daily expense entry and its category.
3. Keep the days in the order given in the input.
4. Add a weekend totals section that sums each category using only the amounts explicitly present in the input.
5. Do not add any category, amount, date, or entry that is not given.
6. If the input leaves something out, write `not given` for that missing item.
7. Deliver the finished table and weekend category totals directly in the output.

Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.

Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.