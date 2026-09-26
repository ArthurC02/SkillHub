---
name: weekly-expense-table-summarizer
description: Organize one week of expense records into a readable table with a category label on each entry, and use it when the user wants a weekly spending summary with category totals at the end of the week.
---

# Weekly Expense Table Summarizer

## Purpose
Turn one week of expense records into a table, label each expense with a category, and add weekend totals by category.

Use this skill when the user gives one week of spending notes and wants them organized into a table with category totals.

## Instructions
1. Read the input and identify the expense entries for one week.
2. Build a table that includes, at minimum, these columns:
   - date
   - item or purpose
   - amount
   - category
3. Keep the categories exactly as they appear in the input when they are already given.
4. If the input does not provide a category for an entry, infer the most reasonable category from the visible text only.
5. Add a weekend summary that totals each category appearing in the week.
6. Show the result as the finished artifact itself in the output.
7. If the input is missing the information needed to do the task, ask only for that missing information.

## Required operating rules
- use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent;
- deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.

## Output shape
- Present the organized spending table first.
- Then present the weekend category totals.
- Keep the output concise and directly usable.

## Notes
- Do not use external information.
- Do not expand beyond the single week in the input.