---
name: expense-reimbursement-reviewer
description: Review each employee expense reimbursement request and return the approval outcome. Use it when you need per-item routing based on attached receipt/invoice, filing age, and amount thresholds.
---

# Expense Reimbursement Reviewer

Use this Skill when you need to review employee reimbursement requests one by one and output the decision for each item.

## What to do

For every reimbursement item in the input, evaluate the rules in this order:

1. If the item does not include an invoice or a receipt, return **退回**.
2. If the reimbursement date is more than 90 days before today, return **退回**.
3. Otherwise, compare the amount:
   - Amount **3000 元以下**: return **直接核准**.
   - Amount **超過 3000 元到 10000 元**: return **需要主管簽核**.
   - Amount **超過 10000 元**: return **需要總經理簽核**.

## Output

- Process each item separately.
- Output one decision per item.
- Do not merge multiple items into a single decision.
- If the input contains multiple reimbursement requests, keep the results in the same order as the input.

## Interpretation notes

- Treat “沒有附發票或收據” as either no invoice or no receipt being attached.
- Treat “距離今天超過 90 天” as the reimbursement date being earlier than today by more than 90 calendar days.
- Treat the amount ranges exactly as written:
  - 3000 元以下 includes 3000.
  - 超過 3000 元到 10000 元 means more than 3000 and up to 10000.
  - 超過 10000 元 means any amount above 10000.

## If input is ambiguous

If a request item does not state whether an invoice or receipt is attached, treat that as **沒有附發票或收據**.

## Final behavior

Return only the decision(s) for the reimbursement items, in the same order as the input.