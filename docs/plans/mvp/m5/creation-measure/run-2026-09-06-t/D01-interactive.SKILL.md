---
name: employee-expense-reimbursement-skill
description: Use this skill when you need to turn the confirmed employee expense reimbursement flow into a reusable workflow for checking invoice completeness, amount thresholds, approval, bookkeeping, and payment notification.
---

# Employee Expense Reimbursement Workflow

Use this skill when you need to follow the confirmed employee expense reimbursement flow exactly as given in the input.

## Instructions

1. Use only what the input contains — never add a date, name, assumption, step or branch the input does not give, and write 'not given' where it is silent.
2. Deliver the finished artifact itself in the output — never a description of the rules, a plan, or a request for access.
3. Start with **收到員工報帳申請**.
4. Check **發票是否齊全**.
5. If **發票是否齊全 = 否**, **退回並要求3天內補件**.
6. If **發票是否齊全 = 是**, check **金額是否超過5000**.
7. If **金額是否超過5000 = 是**, **送經理簽核**.
8. After **送經理簽核**, proceed to **財務直接入帳**.
9. If **金額是否超過5000 = 否**, proceed directly to **財務直接入帳**.
10. After **財務直接入帳**, **寄出付款通知信**.
11. Do not add any other steps, conditions, or outcomes.

## Output

Produce the workflow in the same order as the confirmed flow, with the confirmed step names and branch outcomes only.