---
name: excel-customer-list-dedup-phone-marking
description: 處理 Excel 客戶名單時，用指定欄位去除重複資料，並標記缺少電話的列；當使用者要整理客戶名單、清理重複與找出空白電話時使用。
---

# Goal
處理使用者提供的 Excel 客戶名單，根據使用者指定的欄位去除重複資料，並標記缺少電話的列。

# When to use this skill
當使用者要整理客戶名單、刪除重複資料，或找出電話欄位空白的列時使用。

# Instructions
1. Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
2. Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
3. Read the Excel table the user provided.
4. Identify the column name the user gives for deduplication. If the user does not specify one, use the column the input explicitly names as the customer identifier; if none is given, write 'not given' and state that deduplication cannot be determined from the input.
5. Remove duplicate rows according to the specified deduplication column or columns.
6. Identify rows where the phone column is blank or missing.
7. Mark each row with a missing phone number clearly in the output.
8. Return the result as a practical Excel-ready outcome: either the cleaned table, or a step-by-step Excel action list if the user asked for instructions instead of an edited table.
9. Keep the original meaning of the provided data; do not invent missing values or additional records.
10. If the input does not contain enough information to decide the deduplication key or phone column, state 'not given' for the missing part and stop after reporting the limitation.

# Output requirements
- Show which rows were removed as duplicates.
- Show which rows were marked as missing phone.
- Preserve the remaining rows in a form that can be used directly in Excel.
- If the user asked for a method rather than a transformed table, give concise Excel steps only.

# Constraints
- Do not invent column names or row values.
- Do not ask follow-up questions unless the input itself is missing the information needed to complete the task.
- If the input is incomplete, say what is 'not given' and what can still be done.
