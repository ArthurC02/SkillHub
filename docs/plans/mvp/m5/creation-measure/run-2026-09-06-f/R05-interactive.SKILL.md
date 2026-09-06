---
name: weekly-expense-table
description: 將一週的每日花費整理成表格，為每筆花費標示類別，並在週末彙總各類別合計；適合用來把零散的每日支出轉成可檢視的週報。
---

# Weekly Expense Table

You turn a week of expense notes into a clean Markdown report with two parts:
1. a day-by-day detail table, and
2. a weekend category summary.

Use this skill when the input is a week of spending records or similar daily expense notes that need standardizing, categorizing, and totaling.

## What to do

1. Read the full input as one week of expense entries.
2. Extract each dated/day-labeled expense item.
3. Normalize each line into a detail row with these columns:
   - day
   - item
   - amount
   - category
4. Preserve the original amount values exactly as numbers.
5. Assign a category to every item.
   - If the input already names a category, keep it.
   - If the category is implied by the item text, infer a sensible category from the item meaning.
   - If no reasonable category can be inferred, use `未分類`.
6. Produce a detail table for the whole week.
7. Compute category totals across the full week.
8. Put the totals in a separate summary table grouped by category.
9. Include both tables in the final answer.
10. If the input covers fewer than 7 days, still report only the days present and summarize only those entries.

## Output format

Return Markdown with this structure:

### 明細表
A table with columns:
- 日期/星期
- 項目
- 金額
- 類別

### 類別合計
A table with columns:
- 類別
- 合計金額

## Calculation rules

- Sum amounts by category across all entries in the input.
- Treat values as numeric currency amounts.
- Do not invent extra entries.
- Do not omit entries that are present in the input.
- If a day contains multiple expenses, include one row per expense.
- Keep category names consistent within the same run.

## Quality checks before finishing

- Every expense item in the input appears exactly once in the detail table.
- Every detail row has a category.
- The summary table contains one row per category used in the detail table.
- The category totals match the sum of the matching detail rows.
- The output includes both the detail section and the summary section.