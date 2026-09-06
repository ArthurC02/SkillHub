---
name: weekly-expense-table-summarizer
description: 把一週花費整理成含類別標示的表格，並在週末彙總各類別合計；適合在使用者提供每日花費紀錄時直接生成可讀的週報格式。
---

# weekly-expense-table-summarizer

## Purpose
Turn one week of expense notes into a readable table, label each expense with its category, and add a weekend summary of category totals. Use this skill when the user provides daily spending records and wants them organized without adding any missing items.

## Inputs
- A single week of expense records in free text, rows, or similar structured notes.
- Each expense entry should be taken exactly from the input.
- Category labels should be taken from the input when present.

## Output
Produce:
1. A table covering the expenses in the input.
2. A weekend summary that totals amounts by category.

## Instructions
1. Read the full input once and extract every expense entry that is explicitly present.
2. Preserve the original spending details and dates from the input.
3. For each expense, include its amount and category in a table row.
4. Do not invent expenses, dates, or categories that are not in the input.
5. If the input already provides category labels, use those labels as written.
6. If the input does not provide a category for an expense, leave the category unspecified rather than guessing.
7. After listing the weekly entries, compute a weekend summary by category using only the labeled expenses in the input.
8. Show each category total as the sum of the amounts that were labeled with that category.
9. Keep the presentation clear and directly readable as a table plus summary.
10. Do not ask follow-up questions if the input is complete enough to perform the task in one pass.

## Formatting guidance
- Use a table format that is easy to scan.
- Include columns that make sense for the input, such as day, item, amount, and category.
- Put the weekend category totals immediately after the table.
- If the input spans seven days, include all seven days in the table.

## Constraints
- Stay within the provided week of data.
- Do not add narrative explanations unless needed for clarity.
- Do not pad the output with extra categories or totals beyond what the input supports.