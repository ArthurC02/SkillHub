---
name: clinic-medication-expiry-inventory
description: 盤點診所藥品有效期限時使用；將藥局回報的藥品資料整理成表格，篩出從今天起算 30 天內（含第 30 天）即將過期的品項並提醒優先使用或退回。
---

# Clinic Medication Expiry Inventory

Use this skill when you need to review pharmacy-reported medicine records and identify which items expire within the next 30 days, including day 30, so they can be prioritized for use or return.

## What to do

1. Read the input as a list of medicine records.
2. For each record, use the provided fields:
   - 品名
   - 批號
   - 效期
   - 庫存數量
3. Treat “今天” as the current date at the time the skill is used.
4. Select only the medicines whose expiry date is on or before 30 days from today, including exactly the 30th day.
5. Exclude medicines whose expiry date is after day 30.
6. Present the result as a table.
7. Keep the original fields in the table and add a clear reminder such as “優先使用或退回”.
8. If no medicine meets the 30-day rule, return a table with the required columns and state that no items fall within the 30-day window.

## Output format

Return a table with at least these columns:

- 品名
- 批號
- 效期
- 庫存數量
- 提醒

## Notes

- Only use the data provided in the input.
- Do not invent missing dates, quantities, or medicine names.
- If the expiry date cannot be parsed as a date, mark it as not given in the output and do not include unsupported assumptions.
- Keep the response in the same language as the input.
- Deliver the finished table directly.
- Do not ask follow-up questions unless the input is too incomplete to identify the required fields.
