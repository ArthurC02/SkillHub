---
name: vet-billing-list-compiler
description: Use this skill when you need to turn veterinary clinic visit records into a clean fee list with visit date, service item, quantity, unit price, and a calculated total amount.
---

# Veterinary Clinic Billing List Compiler

Use this skill to convert raw veterinary visit notes, invoices, or line-item records into a structured fee list and compute the total amount.

## What to extract
For each chargeable line item, identify:
- 看診日期 / visit date
- 看診項目 / service or item name
- 數量 / quantity
- 單價 / unit price

If the source includes multiple visits, keep each visit’s items grouped by date.

## Procedure
1. Read the source carefully and locate every chargeable item.
2. Normalize dates into a consistent format used in the source if possible; if the source is ambiguous, preserve the original date text.
3. For each item, record:
   - 日期
   - 項目
   - 數量
   - 單價
   - 小計 = 數量 × 單價
4. If quantity or unit price is missing:
   - Do not invent values.
   - Mark the missing field as unavailable and exclude that line from the total unless the source provides enough information to calculate it.
5. Sum all line subtotals to produce the grand total.
6. Check arithmetic carefully, especially when there are multiple items on the same date.

## Output format
Return a clear table with these columns:
- 看診日期
- 看診項目
- 數量
- 單價
- 小計

Then add:
- 總金額

## Calculation rules
- Use the exact numeric values from the source.
- If the source uses currency symbols, keep them consistent in the output.
- If discounts, taxes, or fees are explicitly listed, include them as separate lines only when they are part of the billing record.
- If the source already provides a total, verify it against your computed total and note any mismatch.

## Quality checks
Before finalizing, confirm that:
- Every listed charge has a date, item, quantity, and unit price when available.
- Each subtotal is correct.
- The grand total equals the sum of all included subtotals.
- No item is counted twice.

## If the source is incomplete
If the input does not contain enough information to build a reliable fee list, say so plainly and list exactly what is missing instead of guessing.
